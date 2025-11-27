package dlq

import (
	"chat/src/clients/redis"
	"chat/src/platform/validation"
	"context"
	"errors"
	"fmt"
	"time"

	redis2 "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const svcBootstrapTimeout = 5 * time.Second

type Letter interface {
	Marshal() ([]byte, error)
	Unmarshal(payload []byte) error
}

type redisConfig struct {
	client   *redis.Client // #readonly
	evalShas redisEvalShas // #readonly
}

type redisEvalShas struct {
	enqueue      string // #readonly
	enqueueMulti string // #readonly
}

type queueConfig struct {
	name string        // #readonly
	ttl  time.Duration // #readonly
}

type Service[T Letter] struct {
	redis  redisConfig    // #readonly
	queue  queueConfig    // #readonly
	logger zerolog.Logger // #readonly
}

type Options struct {
	RedisClient *redis.Client
	QueueName   string        `validate:"required,min=3,max=30,alphanum,lowercase"`
	QueueTTL    time.Duration `validate:"gte=1000000000,lte=600000000000"` // 1s to 10min
	Logger      zerolog.Logger
}

func NewService[T Letter](opts *Options) (*Service[T], error) {
	if err := validation.Instance.Struct(opts); err != nil {
		return nil, fmt.Errorf("can't create DLQ service: invalid options: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), svcBootstrapTimeout)
	defer cancel()

	/*
		-- KEYS[1] = list key
		-- ARGV[1] = value to append
		-- ARGV[2] = expiration in seconds
	*/
	evalShaEnqueue, err := opts.RedisClient.Driver.ScriptLoad(ctx, `
local key     = KEYS[1]
local value   = ARGV[1]
local ttl     = tonumber(ARGV[2])

local existed = redis.call("EXISTS", key)

local new_len = redis.call("RPUSH", key, value)

if existed == 0 then
    redis.call("EXPIRE", key, ttl)
end

return new_len
`).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to init service: can't load Lua script responsible for letter enqueuing: %w", err)
	}

	/*
		-- KEYS[1]  = list key
		-- ARGV[1]  = expiration in seconds
		-- ARGV[2..n] = values to append
	*/
	evalShaEnqueueMulti, err := opts.RedisClient.Driver.ScriptLoad(ctx, `
local key = KEYS[1]
local ttl = tonumber(ARGV[1])

local existed = redis.call("EXISTS", key)

local values = {}
for i = 2, #ARGV do
    values[#values+1] = ARGV[i]
end

local new_len = redis.call("RPUSH", key, unpack(values))

if existed == 0 then
    redis.call("EXPIRE", key, ttl)
end

return new_len
`).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to init service: can't load Lua script responsible for letter enqueuing multi: %w", err)
	}

	return &Service[T]{
		redis: redisConfig{
			client: opts.RedisClient,
			evalShas: redisEvalShas{
				enqueue:      evalShaEnqueue,
				enqueueMulti: evalShaEnqueueMulti,
			},
		},
		queue: queueConfig{
			name: opts.QueueName,
			ttl:  opts.QueueTTL,
		},
		logger: opts.Logger,
	}, nil
}

func (s *Service[T]) Enqueue(ctx context.Context, recipientID string, letter T) (int64, error) {
	payload, err := letter.Marshal()
	if err != nil {
		return 0, fmt.Errorf(
			"failed to marshal letter intended for recipient '%s' from queue '%s': %w",
			recipientID, s.queue.name, err,
		)
	}

	queueLength, err := s.redis.client.Driver.EvalSha(
		ctx,
		s.redis.evalShas.enqueue,
		[]string{s.key(recipientID)},
		payload,
		s.queue.ttl.Seconds(),
	).Int64()
	if err != nil {
		return 0, fmt.Errorf("failed to push letter into queue for recipient '%s' from queue '%s': %w",
			recipientID, s.queue.name, err,
		)
	}

	return queueLength, nil
}

func (s *Service[T]) EnqueueMulti(ctx context.Context, recipientID string, letters []T) (int64, error) {
	argv := make([]any, 0, len(letters)+1)
	argv = append(argv, s.queue.ttl.Seconds())
	for idx, letter := range letters {
		payload, err := letter.Marshal()
		if err != nil {
			return 0, fmt.Errorf(
				"failed to marshal letter at index '%d' intended for recipient '%s' from queue '%s': %w",
				idx, recipientID, s.queue.name, err,
			)
		}
		argv = append(argv, payload)
	}

	queueLength, err := s.redis.client.Driver.EvalSha(
		ctx,
		s.redis.evalShas.enqueueMulti,
		[]string{s.key(recipientID)},
		argv...,
	).Int64()
	if err != nil {
		return 0, fmt.Errorf("failed to push '%d' letters into queue for recipient '%s' from queue '%s': %w",
			len(letters), recipientID, s.queue.name, err,
		)
	}

	return queueLength, nil
}

func (s *Service[T]) Dequeue(ctx context.Context, recipientID string) (T, error) {
	key := s.key(recipientID)

	raw, err := s.redis.client.Driver.LPop(ctx, key).Bytes()
	if err != nil {
		var zero T
		if errors.Is(err, redis2.Nil) {
			return zero, nil
		}
		return zero, fmt.Errorf("failed to dequeue letter for recipient '%s' from queue '%s': %w", recipientID, s.queue.name, err)
	}

	var letter T
	err = letter.Unmarshal(raw)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("failed to unmarshal letter for recipient '%s' from queue '%s': %w", recipientID, s.queue.name, err)
	}

	return letter, nil
}

func (s *Service[T]) DequeueMulti(ctx context.Context, recipientID string, count int) ([]T, error) {
	key := s.key(recipientID)

	rawVals, err := s.redis.client.Driver.LPopCount(ctx, key, count).Result()
	if err != nil {
		if errors.Is(err, redis2.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to dequeue '%d' letters for recipient '%s' from queue '%s': %w", count, recipientID, s.queue.name, err)
	}

	letters := make([]T, 0, len(rawVals))
	for _, raw := range rawVals {
		var letter T
		err = letter.Unmarshal([]byte(raw))
		if err != nil {
			err := s.redis.client.Driver.Del(ctx, key).Err()
			if err != nil {
				s.logger.Warn().Msgf("failed to delete corrupted DLQ '%s': %v", key, err)
			}
			return nil, fmt.Errorf("dlq '%s' corrupted: %w", key, err)
		}
		letters = append(letters, letter)
	}

	return letters, nil
}

func (s *Service[T]) key(recipientID string) string {
	return "dlq:" + s.queue.name + ":" + recipientID
}
