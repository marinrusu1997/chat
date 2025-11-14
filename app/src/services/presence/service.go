package presence

import (
	"chat/src/clients/redis"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
	redis2 "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	sessionKeyFormat     = "presence:user:{%s}:session:%s"
	sessionListKeyFormat = "presence:user:{%s}:sessions"
	lastSeenKeyFormat    = "presence:user:{%s}:last_seen"
)
const (
	sessionTTL     = 60 * time.Second
	sessionListTTL = 90 * time.Second
	lastSeenTTL    = 24 * time.Hour
)
const (
	heartbeatInterval = 30 * time.Second
)
const (
	presenceCacheTTL           = 5 * time.Second
	presenceCacheCapacity      = 10_000
	presenceCacheLoaderTimeout = 100 * time.Millisecond
	lastSeenCacheTTL           = 1 * time.Minute
	lastSeenCacheCapacity      = 5_000
	lastSeenCacheLoaderTimeout = 100 * time.Millisecond
)

type Platform uint8

const (
	PlatformUnknown Platform = iota
	PlatformWeb
	PlatformiOS
	PlatformAndroid
	PlatformDesktop
)

var ErrCacheMiss = errors.New("cache miss")

type Session struct {
	ReplicaHost string
	DeviceID    string
	Platform    Platform
	IP          string
	StartedAt   int64
}

type heartbeats struct {
	mutex        sync.Mutex
	cancelations map[string]context.CancelFunc // key = userID:sessionID
	logger       *zerolog.Logger
}

type Service struct {
	redis         *redis.Client
	isOnlineCache *ttlcache.Cache[string, bool]
	lastSeenCache *ttlcache.Cache[string, int64]
	heartbeats    heartbeats
	logger        *zerolog.Logger
}

func NewService(redisClient *redis.Client, clientLogger *zerolog.Logger) *Service {
	return &Service{
		redis:  redisClient,
		logger: clientLogger,
		isOnlineCache: ttlcache.New[string, bool](
			ttlcache.WithCapacity[string, bool](presenceCacheCapacity),
			ttlcache.WithTTL[string, bool](presenceCacheTTL),
			ttlcache.WithLoader[string, bool](ttlcache.LoaderFunc[string, bool](
				func(cache *ttlcache.Cache[string, bool], userID string) *ttlcache.Item[string, bool] {
					sessionListKey := fmt.Sprintf(sessionListKeyFormat, userID)

					ctx, cancel := context.WithTimeout(context.Background(), presenceCacheLoaderTimeout)
					defer cancel()
					exists, err := redisClient.Driver.Exists(ctx, sessionListKey).Result()
					if err != nil {
						clientLogger.Err(err).Msgf("redis is online check for user '%s' failed", userID)
						return nil
					}

					item := cache.Set(userID, exists == 1, ttlcache.DefaultTTL)
					return item
				},
			)),
			ttlcache.WithDisableTouchOnHit[string, bool](),
		),
		lastSeenCache: ttlcache.New[string, int64](
			ttlcache.WithCapacity[string, int64](lastSeenCacheCapacity),
			ttlcache.WithTTL[string, int64](lastSeenCacheTTL),
			ttlcache.WithLoader[string, int64](ttlcache.LoaderFunc[string, int64](
				func(cache *ttlcache.Cache[string, int64], userID string) *ttlcache.Item[string, int64] {
					lastSeenKey := fmt.Sprintf(lastSeenKeyFormat, userID)

					ctx, cancel := context.WithTimeout(context.Background(), lastSeenCacheLoaderTimeout)
					defer cancel()
					val, err := redisClient.Driver.Get(ctx, lastSeenKey).Result()
					if err != nil {
						if errors.Is(err, redis2.Nil) {
							// key does not exist → offline for > TTL or never connected
							item := cache.Set(userID, 0, ttlcache.DefaultTTL)
							return item
						}
						clientLogger.Err(err).Msgf("redis last seen read for user '%s' failed", userID)
						return nil
					}

					ts, err := strconv.ParseInt(val, 10, 64)
					if err != nil {
						clientLogger.Err(err).Msgf("redis contains invalid last seen value for user '%s': %s", userID, val)
						return nil
					}

					item := cache.Set(userID, ts, ttlcache.DefaultTTL)
					return item
				},
			)),
		),
		heartbeats: heartbeats{
			cancelations: make(map[string]context.CancelFunc),
			logger:       clientLogger,
		},
	}
}

func (s *Service) Start(_ context.Context) error {
	go s.isOnlineCache.Start()
	go s.lastSeenCache.Start()
	return nil
}

func (s *Service) Stop(_ context.Context) {
	s.heartbeats.stopAll()
	s.isOnlineCache.Stop()
	s.lastSeenCache.Stop()
}

func (s *Service) CreateSession(ctx context.Context, userID, sessionID string, session Session) error {
	sessionKey := fmt.Sprintf(sessionKeyFormat, userID, sessionID)
	sessionListKey := fmt.Sprintf(sessionListKeyFormat, userID)
	lastSeenKey := fmt.Sprintf(lastSeenKeyFormat, userID)
	fields := map[string]any{
		"replica_host": session.ReplicaHost,
		"device_id":    session.DeviceID,
		"platform":     strconv.FormatUint(uint64(session.Platform), 10),
		"ip":           session.IP,
		"started_at":   strconv.FormatInt(session.StartedAt, 10),
	}

	// We do not protect against existing sessions with the same ID.
	// It's the caller's responsibility to ensure uniqueness.
	// Worst case, an existing session gets overwritten.
	_, err := s.redis.Driver.TxPipelined(ctx, func(pipe redis2.Pipeliner) error {
		pipe.HSet(ctx, sessionKey, fields)
		pipe.Expire(ctx, sessionKey, sessionTTL)

		pipe.SAdd(ctx, sessionListKey, sessionID)
		pipe.Expire(ctx, sessionListKey, sessionListTTL)

		pipe.Del(ctx, lastSeenKey)

		return nil
	})
	if err != nil {
		return fmt.Errorf("create session with id '%s' for user '%s' failed: %w", sessionID, userID, err)
	}

	// Update caches
	s.isOnlineCache.Set(userID, true, ttlcache.DefaultTTL)
	s.lastSeenCache.Set(userID, 0, ttlcache.DefaultTTL) // cache absence of last seen

	// Start heartbeat to keep session alive.
	s.heartbeats.start(userID, sessionID, s.runHeartbeat)

	return nil
}

func (s *Service) DeleteSession(ctx context.Context, userID, sessionID string) error {
	sessionKey := fmt.Sprintf(sessionKeyFormat, userID, sessionID)
	sessionListKey := fmt.Sprintf(sessionListKeyFormat, userID)
	lastSeenKey := fmt.Sprintf(lastSeenKeyFormat, userID)
	lastSeenTime := time.Now().UnixMilli()
	lastSeenValue := strconv.FormatInt(lastSeenTime, 10)

	// We stop heartbeat, so regardless of whether deletion succeeds or not, it won't be kept alive.
	s.heartbeats.stop(userID, sessionID)

	// Do deletion in a transaction to ensure consistency.
	for {
		err := s.redis.Driver.Watch(ctx, func(tx *redis2.Tx) error {
			sessionCountBefore, err := tx.SCard(ctx, sessionListKey).Result()
			if err != nil {
				return fmt.Errorf("failed to SCARD %s: %w", sessionListKey, err)
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis2.Pipeliner) error {
				pipe.Del(ctx, sessionKey)
				pipe.SRem(ctx, sessionListKey, sessionID)
				if sessionCountBefore == 1 {
					pipe.Set(ctx, lastSeenKey, lastSeenValue, lastSeenTTL)

					s.isOnlineCache.Set(userID, false, ttlcache.DefaultTTL)
					s.lastSeenCache.Set(userID, lastSeenTime, ttlcache.DefaultTTL)
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to transactionally remove session '%s' of user '%s': %w", sessionID, userID, err)
			}
			return nil
		}, sessionListKey)

		if errors.Is(err, redis2.TxFailedErr) {
			continue
		}

		if err != nil {
			return fmt.Errorf("delete session '%s' for user '%s' failed: %w", sessionID, userID, err)
		}
		return nil
	}
}

func (s *Service) IsOnline(userID string) (bool, error) {
	item := s.isOnlineCache.Get(userID)
	if item == nil {
		return false, fmt.Errorf("presence cache miss for user '%s': %w", userID, ErrCacheMiss)
	}
	return item.Value(), nil
}

func (s *Service) LastSeen(userID string) (int64, error) {
	item := s.lastSeenCache.Get(userID)
	if item == nil {
		return 0, fmt.Errorf("last seen cache miss for user '%s': %w", userID, ErrCacheMiss)
	}
	return item.Value(), nil
}

func (s *Service) ListSessions(ctx context.Context, userID string) ([]string, error) {
	sessionListKey := fmt.Sprintf(sessionListKeyFormat, userID)

	sessions, err := s.redis.Driver.SMembers(ctx, sessionListKey).Result()
	if err != nil {
		if errors.Is(err, redis2.Nil) {
			// no sessions → return empty slice, not an error
			return make([]string, 0), nil
		}
		return nil, fmt.Errorf("list sessions for user '%s' failed: %w", userID, err)
	}

	return sessions, nil
}

func (s *Service) GetSession(ctx context.Context, userID, sessionID string) (*Session, error) {
	sessionKey := fmt.Sprintf(sessionKeyFormat, userID, sessionID)

	data, err := s.redis.Driver.HGetAll(ctx, sessionKey).Result()
	if err != nil {
		return nil, fmt.Errorf("get session '%s' for user '%s' failed: %w", sessionID, userID, err)
	}

	if len(data) == 0 {
		return nil, nil //nolint:nilnil // indicate non-existence with (nil, nil)
	}

	sess := &Session{
		ReplicaHost: data["replica_host"],
		DeviceID:    data["device_id"],
		Platform:    PlatformUnknown,
		IP:          data["ip"],
		StartedAt:   0,
	}
	if value, ok := data["platform"]; ok {
		if platform, err := strconv.ParseUint(value, 10, 8); err == nil {
			sess.Platform = Platform(platform)
		} else {
			s.logger.Warn().Msgf("session '%s' for user '%s' has invalid 'platform' field: %s", sessionID, userID, value)
		}
	} else {
		s.logger.Warn().Msgf("session '%s' for user '%s' doesn't have 'platform' field", sessionID, userID)
	}
	if value, ok := data["started_at"]; ok {
		if startedAt, err := strconv.ParseInt(value, 10, 64); err == nil {
			sess.StartedAt = startedAt
		} else {
			s.logger.Warn().Msgf("session '%s' for user '%s' has invalid 'started_at' field: %s", sessionID, userID, value)
		}
	} else {
		s.logger.Warn().Msgf("session '%s' for user '%s' doesn't have 'started_at' field", sessionID, userID)
	}

	return sess, nil
}

func (s *Service) heartbeat(ctx context.Context, userID, sessionID string) error {
	sessionKey := fmt.Sprintf(sessionKeyFormat, userID, sessionID)
	sessionListKey := fmt.Sprintf(sessionListKeyFormat, userID)

	pipe := s.redis.Driver.Pipeline()
	pipe.Expire(ctx, sessionKey, sessionTTL)
	pipe.Expire(ctx, sessionListKey, sessionListTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("heartbeat for session with id '%s' for user '%s' failed: %w", sessionID, userID, err)
	}

	return nil
}

func (s *Service) runHeartbeat(ctx context.Context, userID, sessionID string) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := s.heartbeat(ctx, userID, sessionID)
			if err != nil {
				s.logger.Warn().Err(err).Msg("background heartbeat failed")
			}

		case <-ctx.Done():
			return
		}
	}
}

func (h *heartbeats) start(userID, sessionID string, heartbeater func(ctx context.Context, userID, sessionID string)) {
	heartbeatKey := userID + ":" + sessionID

	h.mutex.Lock()
	_, exists := h.cancelations[heartbeatKey]
	if !exists {
		hbCtx, cancel := context.WithCancel(context.Background())
		h.cancelations[heartbeatKey] = cancel

		go heartbeater(hbCtx, userID, sessionID)
	} else {
		h.logger.Warn().Msgf(
			"heartbeat for session '%s' of user '%s' already exists",
			sessionID, userID,
		)
	}
	h.mutex.Unlock()
}

func (h *heartbeats) stop(userID, sessionID string) {
	heartbeatKey := userID + ":" + sessionID

	h.mutex.Lock()
	if cancel, ok := h.cancelations[heartbeatKey]; ok {
		cancel()
		delete(h.cancelations, heartbeatKey)
	} else {
		h.logger.Warn().Msgf("no heartbeat found for session '%s' of user '%s'", sessionID, userID)
	}
	h.mutex.Unlock()
}

func (h *heartbeats) stopAll() {
	h.mutex.Lock()
	for _, cancel := range h.cancelations {
		cancel()
	}
	h.cancelations = make(map[string]context.CancelFunc)
	h.mutex.Unlock()
}
