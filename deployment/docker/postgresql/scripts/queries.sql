-- queries.sql

-- --
-- User Management
-- --

-- name: CreateUser :one
INSERT INTO "user" (
    email,
    password_hash,
    password_algo,
    name
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM "user"
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM "user"
WHERE email = $1;

-- name: UpdateUserPassword :exec
UPDATE "user"
SET
    password_hash = $1,
    password_algo = $2
WHERE
    id = $3;

-- name: UpdateUserActivity :exec
UPDATE "user"
SET
    last_login_at = $1,
    last_active_at = $2
WHERE
    id = $3;

-- name: DeleteUser :exec
DELETE FROM "user"
WHERE id = $1;


-- --
-- Device & Signal Key Management
-- --

-- name: CreateDevice :one
INSERT INTO chatting_device (
    user_id,
    name,
    role,
    fingerprint,
    last_seen_at
    -- expires_at is set by a trigger
) VALUES (
             $1, $2, $3, $4, NOW()
         )
RETURNING *;

-- name: GetUserActiveDevices :many
SELECT * FROM chatting_device
WHERE user_id = $1 AND expires_at > NOW()
ORDER BY role, created_at;

-- name: UpdateDeviceLastSeen :one
UPDATE chatting_device
SET last_seen_at = NOW()
WHERE id = $1 AND user_id = $2
RETURNING expires_at;

-- name: CreateDeviceSignalKeys :one
INSERT INTO device_signal_keys (
    device_id,
    user_id,
    identity_key,
    signed_pre_key_id,
    signed_pre_key,
    signed_pre_key_signature
) VALUES (
             $1, $2, $3, $4, $5, $6
         )
RETURNING *;

-- name: AddOneTimePreKeys :copyfrom
-- This uses sqlc's :copyfrom for efficient bulk inserts.
INSERT INTO one_time_pre_key (
    device_id,
    public_key
) VALUES (
             $1, $2
         );

-- name: GetUserSignalPreKeyBundle :many
-- This demonstrates how to call the custom function you defined.
SELECT * FROM get_user_signal_pre_key_bundle($1, $2, $3);


-- --
-- Chat Management
-- --

-- name: CreateChat :one
INSERT INTO chat (
    type,
    visibility,
    post_policy,
    created_by,
    name,
    tags,
    topic,
    description
) VALUES (
             $1, $2, $3, $4, $5, $6, $7, $8
         )
RETURNING *;

-- name: GetChatByID :one
SELECT * FROM chat
WHERE id = $1;

-- name: SearchPublicGroupChatsByName :many
-- This uses the Full-Text Search index on the `name` column.
SELECT id, name, topic, description, tags FROM chat
WHERE
    name_fts @@ to_tsquery('english', $1)
  AND type = 'group'
  AND visibility = 'public'
  AND status = 'active'
ORDER BY
    ts_rank(name_fts, to_tsquery('english', $1)) DESC
LIMIT 20;

-- name: UpdateChatTopic :exec
UPDATE chat
SET topic = $1
WHERE id = $2;


-- --
-- Chat Participant Management
-- --

-- name: AddChatParticipant :one
INSERT INTO chat_participant (
    chat_id,
    user_id,
    chat_type,
    role,
    permissions_bitmask
) VALUES (
             $1, $2, $3, $4, $5
         )
RETURNING *;

-- name: GetChatParticipant :one
SELECT * FROM chat_participant
WHERE chat_id = $1 AND user_id = $2;

-- name: GetUserChats :many
-- Gets all non-left chats for a given user.
SELECT c.*
FROM chat c
         JOIN chat_participant cp ON c.id = cp.chat_id
WHERE
    cp.user_id = $1
  AND cp.left_at IS NULL
ORDER BY cp.is_pinned DESC, cp.last_read_at ASC NULLS FIRST;

-- name: GetChatParticipants :many
-- Gets all current participants of a chat.
SELECT
    u.id as user_id,
    u.name,
    cp.role,
    cp.custom_nickname,
    cp.joined_at
FROM chat_participant AS cp
         JOIN "user" AS u ON cp.user_id = u.id
WHERE
    cp.chat_id = $1
  AND cp.left_at IS NULL
ORDER BY cp.role, cp.joined_at;

-- name: UpdateParticipantRole :one
UPDATE chat_participant
SET
    role = $1,
    permissions_bitmask = $2
WHERE
    chat_id = $3 AND user_id = $4
RETURNING *;

-- name: UpdateParticipantLastRead :exec
UPDATE chat_participant
SET
    last_read_message_id = $1,
    last_read_at = NOW()
WHERE
    chat_id = $2 AND user_id = $3;

-- name: RemoveChatParticipant :exec
DELETE FROM chat_participant
WHERE chat_id = $1 AND user_id = $2;