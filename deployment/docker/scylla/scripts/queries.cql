-- =================================================================
--  ScyllaDB Production-Ready Queries for Chat Application
-- =================================================================
-- Version: 2.1 (Last Updated: September 18, 2025)
-- This file contains queries that are compatible with the latest schema (init.cql).
-- Placeholders like 'uuid_placeholder' should be replaced by your application.

USE chat_app;

-- ================================================================
--  Section 1: Core Messaging & Chat Lifecycle
-- ================================================================
-- These queries handle the fundamental operations of sending and
-- receiving messages. They operate on the TWCS tables.
-- ================================================================

-- @fixme use timeouts for queries: https://docs.scylladb.com/manual/stable/cql/cql-extensions.html#using-timeout

-- Query 1: Send a New Message
-- Purpose: An idempotent dual-write to insert a new message.
-- When to use: When a user sends any type of message.
-- Go App Logic: Your application must generate the `message_id` (TIMEUUID) before
--               the insert for idempotency. The two INSERTs below cannot be
--               executed in a `LOGGED BATCH` to ensure atomicity, because they use different partition keys.
--               This can be solved with Asynchronous Indexing with a Queue or CDC (The Event-Driven Approach).
-- Write 1: Main message table
-- name: CreateMessageInChat :exec
INSERT INTO messages_by_chat (
    chat_id, message_id, day_bucket, sender_id,
    message_type, content, client_metadata,
    reply_to_message_id, forwarded_from_message_id,
    edited_at, deleted_at, expires_at, scheduled_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    USING CONSISTENCY LOCAL_QUORUM;
-- Write 2: Manual index table
-- name: CreateMessageInIndex :exec
INSERT INTO messages_by_id (
    chat_id, message_id, sender_id,
    message_type, content,
    reply_to_message_id, forwarded_from_message_id,
    edited_at, deleted_at, expires_at,
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    USING CONSISTENCY LOCAL_QUORUM;

-- Query 2: Fetch a Page of Recent Messages
-- Purpose: Selects the latest N messages from a chat partition.
-- When to use: When a user opens a chat screen.
-- Go App Logic: This is a fast, single-partition query. The `day_bucket` should be the
--               current date for fetching the most recent messages.
-- name: GetRecentMessages :many
SELECT * FROM messages_by_chat
WHERE chat_id = ? AND day_bucket = ?
ORDER BY message_id DESC
LIMIT ?
    USING CONSISTENCY LOCAL_ONE;

-- Query 3: Fetch Older Messages (Infinite Scroll)
-- Purpose: Selects N messages before a specific message_id, enabling pagination.
-- When to use: When a user scrolls up in a chat to load older history.
-- Go App Logic: Pass the `day_bucket` and `message_id` of the last seen message
--               to paginate backwards in time.
-- name: GetOlderMessages :many
SELECT * FROM messages_by_chat
WHERE chat_id = ? AND day_bucket = ? AND message_id < ?
ORDER BY message_id DESC
LIMIT ?
    USING CONSISTENCY LOCAL_ONE;

-- name: GetAverageMessageLength :one
SELECT average_message_length(content)
FROM messages_by_chat
WHERE chat_id = ? AND day_bucket = ?;

-- Query 4: Fetch a Single Message by ID
-- Purpose: A fast, direct lookup of a message using the manual index.
-- When to use: When you need to fetch a specific message, e.g., for a reply preview or after a search.
-- Go App Logic: Use a high consistency level to ensure you get the most up-to-date
--               version of the message.
-- name: GetMessageByID :one
SELECT * FROM messages_by_id
WHERE message_id = ?
    USING CONSISTENCY LOCAL_QUORUM;

-- Query 5: Edit a Message (The "Overwrite" Pattern)
-- Purpose: Updates the content and status of a message.
-- When to use: When a user edits a message they previously sent.
-- Go App Logic: This is a dual-write that overwrites the existing row data.
--               It is the correct, TWCS-safe way to perform an "update".
-- Write 1: Overwrite in the main message table
-- name: EditMessageInChat :exec
UPDATE messages_by_chat
SET content = ?, edited_at = ?
WHERE chat_id = ? AND day_bucket = ? AND message_id = ?
IF EXISTS
    USING CONSISTENCY LOCAL_QUORUM;
-- Write 2: Overwrite in the index table
-- name: EditMessageInIndex :exec
UPDATE messages_by_id
SET content = ?, edited_at = ?
WHERE message_id = ?
IF EXISTS
    USING CONSISTENCY LOCAL_QUORUM;

-- Query 6: Delete a Message (The "Soft Delete" Pattern)
-- Purpose: Marks a message as deleted without using a `DELETE` command.
-- When to use: When a user deletes a message.
-- Go App Logic: This is the TWCS-safe "soft delete". Your application's read logic must
--               now filter out and hide any messages where `deleted_at IS NOT NULL`.
-- Write 1: Soft delete in the main table
-- name: SoftDeleteMessageInChat :exec
UPDATE messages_by_chat
SET deleted_at = ?
WHERE chat_id = ? AND day_bucket = ? AND message_id = ?
IF EXISTS
    USING CONSISTENCY LOCAL_QUORUM;
-- Write 2: Soft delete in the index table
-- name: SoftDeleteMessageInIndex :exec
UPDATE messages_by_id
SET deleted_at = ?
WHERE message_id = ?
IF EXISTS
    USING CONSISTENCY LOCAL_QUORUM;

-- ================================================================
--  Section 2: User Inbox & State Management (LCS Tables)
-- ================================================================
-- These queries operate on LCS tables, where `DELETE` is safe.
-- ================================================================

-- Query 7: Fetch User's Inbox
-- Purpose: Gets a user's list of chats, sorted by the most recent activity.
-- When to use: When loading the main screen of the application.
-- name: GetUserInbox :many
SELECT * FROM inbox_by_user
WHERE user_id = ?
LIMIT ?
    USING CONSISTENCY LOCAL_ONE;

-- Query 8: Update Inbox on New Message (UPSERT)
-- Purpose: Updates a chat's entry in a user's inbox.
-- When to use: For every participant of a chat when a new message is sent.
-- name: UpdateUserInbox :exec
INSERT INTO inbox_by_user (
   user_id,
   chat_id, chat_display_name, chat_avatar_url, chat_type,
   last_message_id, last_message_content_preview, last_message_sender_id, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    USING CONSISTENCY LOCAL_ONE;

-- Query 9: Mark a Chat as Read
-- Purpose: Updates a user's read pointer for a specific chat.
-- When to use: When a user opens a chat or scrolls to the bottom.
-- name: MarkChatAsRead :exec
INSERT INTO chat_read_state (user_id, chat_id, last_read_message_id, last_read_at)
VALUES (?, ?, ?, ?)
    USING CONSISTENCY LOCAL_ONE;

-- Query 10: Fetch All Unread States
-- Purpose: Gets all read pointers for a user, used to calculate unread status on the client.
-- When to use: On application startup to determine which chats in the inbox have unread messages.
-- name: GetUserReadStates :many
SELECT * FROM chat_read_state
WHERE user_id = ?
    USING CONSISTENCY LOCAL_ONE;

-- Query 11: Leave a Chat
-- Purpose: Removes a user from a chat's inbox and clears their read state.
-- When to use: When a user explicitly leaves a group or deletes a conversation.
-- Note: These don't have to be run in a `LOGGED BATCH`, because they operate on different partition keys.
-- Application logic must ensure both complete successfully.
-- name: DeleteInboxEntry :exec
DELETE FROM inbox_by_user
WHERE user_id = ? AND updated_at = ? AND chat_id = ?
    USING CONSISTENCY LOCAL_QUORUM;
-- name: DeleteChatReadState :exec
DELETE FROM chat_read_state
WHERE user_id = ? AND chat_id = ?
    USING CONSISTENCY LOCAL_QUORUM;

-- Prevents an old, out-of-order event from overwriting the inbox with stale data.
UPDATE inbox_by_user
SET last_message_content_preview = ?, last_message_id = ? -- new message id
WHERE user_id = ? AND updated_at = ? AND chat_id = ?
IF last_message_id = ? -- old message id
    USING CONSISTENCY LOCAL_QUORUM;

-- ================================================================
--  Section 3: In-Chat Features & Experience
-- ================================================================

-- Query 12 & 13: Add / Remove a Reaction
-- Purpose: Adds or removes a user's reaction to a message.
-- When to use: When a user taps the react button on a message.
-- name: AddReaction :exec
INSERT INTO reactions_by_message (message_id, reactor_id, reaction_emoji, reacted_at)
VALUES (?, ?, ?, ?)
    USING CONSISTENCY LOCAL_ONE;
-- name: RemoveReaction :exec
DELETE FROM reactions_by_message
WHERE message_id = ? AND reactor_id = ? AND reaction_emoji = ?
   USING CONSISTENCY LOCAL_ONE;

-- Query 14: Fetch All Reactions for a Message
-- Purpose: Gets all reactions for a given message in a single partition read.
-- When to use: When displaying a message to show who has reacted.
-- name: GetReactionsForMessage :many
SELECT * FROM reactions_by_message
WHERE message_id = ?
    USING CONSISTENCY LOCAL_ONE;

-- Query 15 & 16: Pin / Unpin a Message
-- Purpose: Adds or removes a message from the pinned list in a chat.
-- When to use: When a chat admin pins or unpins a message.
-- name: PinMessage :exec
INSERT INTO pinned_messages_by_chat (chat_id, pinned_at, message_id, pinner_id, message_content_preview)
VALUES (?, ?, ?, ?, ?)
    USING CONSISTENCY LOCAL_QUORUM;
-- name: UnpinMessage :exec
DELETE FROM pinned_messages_by_chat
WHERE chat_id = ? AND pinned_at = ? AND message_id = ?
    USING CONSISTENCY LOCAL_QUORUM;

-- Query 17: Fetch Pinned Messages
-- Purpose: Gets all pinned messages for a chat, sorted newest first.
-- When to use: When a user views the pinned messages bar in a chat.
-- name: GetPinnedMessages :many
SELECT * FROM pinned_messages_by_chat
WHERE chat_id = ?
    USING CONSISTENCY LOCAL_ONE;

-- Query 18: View All Media/Files in a Chat
-- Purpose: Gets a paginated list of attachments for a chat, newest first.
-- When to use: When a user opens the "Media" or "Files" section of a chat.
-- name: GetChatMedia :many
SELECT attachment_id, sender_id, filename, thumbnail_url, uploaded_at
FROM attachments_by_chat
WHERE chat_id = ?
LIMIT ?
    USING CONSISTENCY LOCAL_ONE;

-- Query 19: Look up Attachment Details
-- Purpose: A fast, direct lookup of attachment metadata by its ID.
-- When to use: When a user clicks on an attachment to view or download it.
-- name: GetAttachmentDetails :one
SELECT *
FROM attachments_by_id
WHERE attachment_id = ?
    USING CONSISTENCY LOCAL_ONE;

-- ================================================================
--  Section 4: User, Social & Discovery Features
-- ================================================================

-- Query 20: Fetch User Profile
-- This query is NOT supported by the ScyllaDB schema.
-- Per the architectural notes, this relational data is correctly handled by a separate PostgreSQL database.

-- Query 21: Fetch User Mentions
-- Purpose: Retrieves recent mentions for a user.
-- When to use: When a user opens their "Mentions" or "Notifications" tab.
-- Go App Logic: The application should query the last few day_buckets (e.g., today, yesterday) to find recent activity.
-- name: GetUserMentions :many
SELECT * FROM mentions_by_user
WHERE mentioned_user_id = ? AND day_bucket IN (?, ?)
ORDER BY message_id DESC
LIMIT ?
    USING CONSISTENCY LOCAL_ONE;

-- Purpose: Counts the number of messages sent by each user in a chat for a specific day.
-- When to use: For an internal analytics dashboard.
-- name: GetUserActivityInChat :many
SELECT sender_id, COUNT(*) as message_count
FROM messages_by_chat
WHERE chat_id = ? AND day_bucket = ?
GROUP BY sender_id;

-- Query 22: Fetch Chat Members
-- This query is NOT supported by the ScyllaDB schema.
-- Per the architectural notes, chat membership and roles are relational data handled by PostgreSQL.

-- Query 23 & 24: Message Search (Global or In-Chat)
-- This query is NOT supported directly by ScyllaDB for keyword matching.
-- The correct architecture is to use a dedicated search index (e.g., Elasticsearch).
-- The workflow is:
--   1. The client sends a search query to the application backend.
--   2. The backend queries Elasticsearch to find matching message_ids.
--   3. The backend uses the returned message_ids to fetch full message details from ScyllaDB using Query #4.