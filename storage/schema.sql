PRAGMA foreign_keys = ON;

-- User table
CREATE TABLE IF NOT EXISTS user
(
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    -- credentials
    email                   TEXT NOT NULL UNIQUE CHECK (
                                email LIKE '%_@_%._%' AND
                                LENGTH(email) - LENGTH(REPLACE(email, '@', '')) = 1 AND
                                SUBSTR(email, 1, INSTR(email, '.') - 1) NOT GLOB '*[^@0-9a-zA-Z]*' AND
                                SUBSTR(email, INSTR(email, '.') + 1) NOT GLOB '*[^a-zA-Z]*'
                            ),
    password_hash           TEXT NOT NULL CHECK (length(password_hash) >= 50 AND length(password_hash) <= 200),
    password_algo           INTEGER NOT NULL CHECK (password_algo BETWEEN 0 AND 100),
    password_updated_at     TIMESTAMP CHECK (password_updated_at IS NULL OR (password_updated_at >= last_active_at AND password_updated_at <= CURRENT_TIMESTAMP)),
    -- PII
    name                    TEXT NOT NULL CHECK (length(name) >= 2 AND length(name) <= 100),
    -- activity
    last_login_at           TIMESTAMP CHECK (last_login_at IS NULL OR (last_login_at > created_at AND last_login_at <= CURRENT_TIMESTAMP)),
    last_active_at          TIMESTAMP CHECK (last_active_at IS NULL OR (last_active_at >= last_login_at AND last_active_at <= CURRENT_TIMESTAMP)),
    created_at              TIMESTAMP CHECK (created_at <= CURRENT_TIMESTAMP) NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

-- Chat table
CREATE TABLE IF NOT EXISTS chat
(
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    -- enums
    type                TEXT NOT NULL CHECK (type IN ('direct', 'group', 'self', 'thread')),
    visibility          TEXT NOT NULL CHECK (visibility IN ('public', 'private', 'secret')),
    post_policy         TEXT NOT NULL CHECK (post_policy IN ('all', 'admins', 'owner')),
    status              TEXT NOT NULL CHECK (status IN ('active', 'archived', 'locked')) DEFAULT 'active',
    moderation_policy   TEXT NOT NULL CHECK (moderation_policy IN ('none', 'flagged_review', 'auto_delete')) DEFAULT 'none',
    -- settings / presentation
    name                TEXT CHECK (name IS NULL OR (length(name) >= 2 AND length(name) <= 100)),
    topic               TEXT CHECK (topic IS NULL OR (length(topic) >= 2 AND length(topic) <= 100)),
    description         TEXT CHECK (description IS NULL OR (length(description) >= 2 AND length(description) <= 500)),
    settings            TEXT CHECK (settings IS NULL OR (length(settings) >= 1 AND length(settings) <= 500 AND json_valid(settings))),
    -- authorship / lineage
    created_by          INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
    created_at          TIMESTAMP NOT NULL CHECK (created_at <= CURRENT_TIMESTAMP) DEFAULT CURRENT_TIMESTAMP,
    parent_id           INTEGER REFERENCES chat(id) ON DELETE CASCADE,
    expires_at          TIMESTAMP CHECK (expires_at IS NULL OR (expires_at > CURRENT_TIMESTAMP)),
    -- group thread toggle (global per chat)
    threads_enabled     BOOLEAN NOT NULL DEFAULT FALSE -- @fixme needs to also be set per message
) STRICT;

-- User Participant junction table
CREATE TABLE IF NOT EXISTS chat_participant (
    -- identifiers & relations
    chat_id                 INTEGER NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id                 INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    -- membership & roles
    role                    TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'moderator', 'member', 'guest', 'bot')) DEFAULT 'member',
    permissions_bitmask     INTEGER NOT NULL CHECK (permissions_bitmask >= 0) DEFAULT 0,
    joined_at               TIMESTAMP NOT NULL CHECK (joined_at <= CURRENT_TIMESTAMP) DEFAULT CURRENT_TIMESTAMP,
    rejoined_at             TIMESTAMP CHECK (rejoined_at IS NULL OR (rejoined_at > joined_at AND rejoined_at <= CURRENT_TIMESTAMP)),
    -- status & moderation
    left_at                 TIMESTAMP CHECK (left_at IS NULL OR (left_at > joined_at AND left_at <= CURRENT_TIMESTAMP)),
    banned_reason_code      TEXT CHECK (banned_reason_code IN ('spam', 'abuse', 'harassment', 'scam', 'policy_violation', 'other')),
    banned_reason_note      TEXT CHECK (length(banned_reason_note) >= 1 AND length(banned_reason_note) <= 200),
    ban_type                TEXT CHECK (ban_type IN ('temporary', 'permanent', 'shadow')),
    banned_until            TIMESTAMP CHECK (banned_until IS NULL OR (banned_until > CURRENT_TIMESTAMP)),
    banned_by               INTEGER REFERENCES chat_participant(user_id) ON DELETE RESTRICT,
    -- invitations
    invited_by              INTEGER REFERENCES chat_participant(user_id) ON DELETE RESTRICT,
    invited_at              TIMESTAMP CHECK (invited_at IS NULL OR (invited_at < joined_at)),
    -- read tracking & activity
    last_read_message_id    INTEGER REFERENCES message(id) ON DELETE RESTRICT,
    last_read_at            TIMESTAMP CHECK (last_read_at IS NULL OR (last_read_at > joined_at AND last_read_at <= CURRENT_TIMESTAMP)),
    -- presence_status      TEXT NOT NULL CHECK (presence_status IN ('online', 'offline', 'idle', 'dnd')) DEFAULT 'offline', @fixme will be stored in Redis
    -- notifications & preferences
    muted_until             TIMESTAMP CHECK (muted_until IS NULL OR (muted_until > CURRENT_TIMESTAMP)),
    notification_level      TEXT NOT NULL CHECK (notification_level IN ('all', 'mentions_only', 'important_only', 'none')) DEFAULT 'all',
    custom_nickname         TEXT CHECK (length(custom_nickname) >= 1 AND length(custom_nickname) <= 50),
    color_theme             TEXT CHECK (length(color_theme) >= 1 AND length(color_theme) <= 50),
    settings                TEXT CHECK (settings IS NULL OR (length(settings) >= 1 AND length(settings) <= 500 AND json_valid(settings))),
    -- pinning & tagging
    is_pinned               BOOLEAN NOT NULL DEFAULT FALSE,
    last_pinned_message_id  INTEGER REFERENCES message(id) ON DELETE SET NULL,
    tagset                  TEXT CHECK (tagset IS NULL OR (length(tagset) >= 1 AND length(tagset) <= 150 AND json_valid(tagset))),

    PRIMARY KEY (user_id, chat_id),

    ------------------------------------------------------
    -- Compound static constraints
    ------------------------------------------------------
    -- Permissions per role check (see chat_permissions.md)
    CHECK (
        (role = 'guest' AND permissions_bitmask >= 0 AND permissions_bitmask <= 0x0000000000000000) OR
        (role = 'bot' AND permissions_bitmask >= 0 AND permissions_bitmask <= 0xfffffff800000000) OR
        (role = 'member' AND permissions_bitmask >= 0 AND permissions_bitmask <= 0xfffffff800000000) OR
        (role = 'moderator' AND permissions_bitmask >= 0 AND permissions_bitmask <= 0xffffffffffffc000) OR
        (role = 'admin' AND permissions_bitmask >= 0 AND permissions_bitmask <= 0xfffffffffffffff0) OR
        (role = 'owner' AND permissions_bitmask = 0xffffffffffffffff)
    ),

    -- If ban_type is 'temporary', banned_until must be set
    CHECK (ban_type IS DISTINCT FROM 'temporary' OR banned_until IS NOT NULL),

    -- If ban_type is 'permanent', banned_until must be NULL
    CHECK (ban_type IS DISTINCT FROM 'permanent' OR banned_until IS NULL),

    -- If banned_reason_note is provided, banned_reason_code must be set
    CHECK (banned_reason_note IS NULL OR banned_reason_code IS NOT NULL)
) STRICT WITHOUT ROWID;

-- Message table
CREATE TABLE IF NOT EXISTS message
(
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id     INTEGER NOT NULL,
    user_id     INTEGER NOT NULL,
    content     TEXT NOT NULL CHECK(length(content) <= 2000), -- @fixme encrypt it later on
    status      TEXT NOT NULL CHECK(status IN ('active', 'edited')) DEFAULT 'active',
    edited_at   TIMESTAMP CHECK(edited_at IS NULL OR (edited_at > sent_at AND edited_at < expires_at)),
    expires_at  TIMESTAMP CHECK(expires_at IS NULL OR expires_at > sent_at), -- @fixme implement it later on (retention policy)
    sent_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (chat_id)       REFERENCES chat(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id)       REFERENCES user(id) ON DELETE CASCADE
) STRICT;

-- Read_Receipts table
CREATE TABLE IF NOT EXISTS read_receipt (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id    INTEGER NOT NULL,
    user_id       INTEGER NOT NULL,
    read_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, -- @fixme validate it read after message was sent

    UNIQUE(message_id, user_id),
    FOREIGN KEY (message_id)    REFERENCES message(id)  ON DELETE CASCADE,
    FOREIGN KEY (user_id)       REFERENCES user(id)     ON DELETE CASCADE
) STRICT;

-- Audit_Log table
CREATE TABLE IF NOT EXISTS audit_log (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL,
    action       TEXT NOT NULL CHECK(action IN (
                                                'message_edited',
                                                'chat_created',
                                                'chat_updated',
                                                'chat_joined',
                                                'chat_left')
        ),
    entity_type  TEXT NOT NULL CHECK(entity_type IN ('chat', 'message', 'chat_member')),
    entity_id    INTEGER NOT NULL CHECK(entity_id > 0),
    old_value    TEXT CHECK(length(old_value) <= 1000), -- Max 1000 characters
    new_value    TEXT CHECK(length(new_value) <= 1000), -- Max 1000 characters
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (user_id) REFERENCES user(id) ON DELETE CASCADE
) STRICT;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_email                    ON user(email);

CREATE INDEX IF NOT EXISTS chat_name                    ON chat(name COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS chat_parent_idx              ON chat(parent_id);
CREATE INDEX IF NOT EXISTS chat_type_idx                ON chat(type);
CREATE INDEX IF NOT EXISTS chat_status_idx              ON chat(status);

CREATE INDEX IF NOT EXISTS idx_message_chat_id          ON message(chat_id);
CREATE INDEX IF NOT EXISTS idx_message_user_id          ON message(user_id);
CREATE INDEX IF NOT EXISTS idx_message_sent_at          ON message(sent_at);
CREATE INDEX IF NOT EXISTS idx_message_chat_id_sent_at  ON message(chat_id, sent_at);

CREATE INDEX IF NOT EXISTS idx_chat_member_user_id      ON chat_member(user_id);
CREATE INDEX IF NOT EXISTS idx_chat_member_chat_id      ON chat_member(chat_id);
CREATE INDEX IF NOT EXISTS idx_read_receipt_message_id  ON read_receipt(message_id);

-- Triggers
--- User
CREATE TRIGGER user_immutable_columns
    BEFORE UPDATE OF email, name, created_at ON user
    FOR EACH ROW
BEGIN
    SELECT RAISE(
                   ABORT,
                   'immutable columns changed:' ||
                   CASE WHEN NEW.email      IS NOT OLD.email        THEN ' email'       ELSE '' END ||
                   CASE WHEN NEW.name       IS NOT OLD.name         THEN ' name'        ELSE '' END ||
                   CASE WHEN NEW.created_at IS NOT OLD.created_at   THEN ' created_at'  ELSE '' END
           );
END;

CREATE TRIGGER user_password_insert
    BEFORE INSERT ON user
    FOR EACH ROW
BEGIN
    SELECT
        CASE
            WHEN NEW.password_updated_at IS NOT NULL
                THEN RAISE(ABORT, 'password_updated_at must be NULL on creation')
        END;
END;
CREATE TRIGGER user_password_update
    BEFORE UPDATE OF password_hash, password_algo ON user
    FOR EACH ROW
BEGIN
    SELECT
        CASE
            WHEN NEW.password_hash = OLD.password_hash AND NEW.password_algo != OLD.password_algo
                THEN RAISE(ABORT, 'password_hash must also change when password_algo changes')
            WHEN NEW.password_hash = OLD.password_hash
                THEN RAISE(ABORT, 'password_hash must be different from old value')
            WHEN NEW.password_algo <= OLD.password_algo
                THEN RAISE(ABORT, 'password_algo must increase')
        END;
    SET NEW.password_updated_at = CURRENT_TIMESTAMP;
END;

--- Chat
-- ============================================================
-- 1) Read-only when ARCHIVED
--    If a row is archived, it cannot be updated anymore.
-- ============================================================
CREATE TRIGGER chat_readonly_if_archived
    BEFORE UPDATE ON chat
    FOR EACH ROW
    WHEN OLD.status = 'archived'
BEGIN
    SELECT RAISE(ABORT, 'chat is archived and read-only');
END;

-- ============================================================
-- 2) Immutable columns: type, name, created_by, created_at, parent_id, threads_enabled
--    Build a single message listing all changed immutables.
-- ============================================================
CREATE TRIGGER chat_immutables_guard
    BEFORE UPDATE OF type, name, created_by, created_at, parent_id, threads_enabled ON chat
    FOR EACH ROW
BEGIN
    SELECT RAISE(
                    ABORT,
                    'immutable columns changed:' ||
                    CASE WHEN NEW.type              IS NOT OLD.type             THEN ' type'            ELSE '' END ||
                    CASE WHEN NEW.name              IS NOT OLD.name             THEN ' name'            ELSE '' END ||
                    CASE WHEN NEW.created_by        IS NOT OLD.created_by       THEN ' created_by'      ELSE '' END ||
                    CASE WHEN NEW.created_at        IS NOT OLD.created_at       THEN ' created_at'      ELSE '' END ||
                    CASE WHEN NEW.parent_id         IS NOT OLD.parent_id        THEN ' parent_id'       ELSE '' END ||
                    CASE WHEN NEW.threads_enabled   IS NOT OLD.threads_enabled  THEN ' threads_enabled' ELSE '' END
                );
END;

-- ============================================================
-- 3) Type-specific constraints on INSERT/UPDATE
--    Direct, Self, Group, Thread.
--    Parent-related checks done here for threads.
-- ============================================================
CREATE TRIGGER chat_visibility_rules_update
    BEFORE UPDATE OF visibility ON chat
    FOR EACH ROW
BEGIN
    SELECT
        CASE
            WHEN (OLD.type = 'direct' OR OLD.type = 'self')
                THEN RAISE(ABORT, 'direct or self chats cannot change visibility')

            WHEN OLD.type = 'thread' AND (
                (CASE (SELECT visibility FROM chat WHERE id = OLD.parent_id)
                     WHEN 'private' THEN (NEW.visibility IN ('private', 'secret'))
                     WHEN 'secret'  THEN NEW.visibility = 'secret'
                    END) != TRUE
                )
                THEN RAISE(ABORT, 'visibility of thread chat cannot exceed parent visibility')
            END;
END;

CREATE TRIGGER chat_post_policy_update
    BEFORE UPDATE OF post_policy ON chat
    FOR EACH ROW
BEGIN
    SELECT
        CASE
            WHEN (OLD.type = 'direct' OR OLD.type = 'self')
                THEN RAISE(ABORT, 'direct or self chats cannot change post_policy')

            WHEN OLD.type = 'thread' AND (
                (CASE (SELECT post_policy FROM chat WHERE id = OLD.parent_id)
                    WHEN 'all'      THEN NEW.post_policy = 'all'
                    WHEN 'admins'   THEN (NEW.post_policy IN ('all', 'admins'))
                    WHEN 'owner'    THEN (NEW.post_policy IN ('all', 'owner'))
                    END) != TRUE
                )
                THEN RAISE(ABORT, 'post_policy of thread chat cannot exceed parent post_policy')
            END;
END;

CREATE TRIGGER chat_status_update
    BEFORE UPDATE OF status ON chat
    FOR EACH ROW
BEGIN
    SELECT
        CASE
            WHEN ((OLD.type = 'direct' OR OLD.type = 'self') AND NEW.status = 'archived')
                THEN RAISE(ABORT, 'direct or self chats cannot be archived')

            WHEN OLD.type = 'thread' AND (
                (CASE (SELECT status FROM chat WHERE id = OLD.parent_id)
                    WHEN 'locked' THEN (NEW.status IN ('archived', 'locked'))
                    END) != TRUE
                )
                THEN RAISE(ABORT, 'status of thread chat cannot exceed parent status')
            END;
END;

CREATE TRIGGER chat_moderation_policy_update
    BEFORE UPDATE OF moderation_policy ON chat
    FOR EACH ROW
BEGIN
    SELECT
        CASE
            WHEN (OLD.type = 'direct' OR OLD.type = 'self')
                THEN RAISE(ABORT, 'direct or self chats cannot change moderation_policy')

            WHEN OLD.type = 'thread' AND (
                (CASE (SELECT moderation_policy FROM chat WHERE id = OLD.parent_id)
                    WHEN 'flagged_review'   THEN NEW.moderation_policy = 'auto_delete'
                    WHEN 'auto_delete'      THEN NEW.moderation_policy = 'auto_delete'
                    END) != TRUE
                )
                THEN RAISE(ABORT, 'moderation_policy of thread chat cannot exceed parent moderation_policy')
            END;
END;

CREATE TRIGGER chat_topic_update
    BEFORE UPDATE OF topic ON chat
    FOR EACH ROW
BEGIN
    SELECT
        CASE
            WHEN (OLD.type IN ('direct', 'self', 'thread'))
                THEN RAISE(ABORT, 'direct or self chats cannot change topic')

            WHEN OLD.type = 'group' AND NEW.topic IS NULL
                THEN RAISE(ABORT, 'group chats must have a topic')
            END;
END;

CREATE TRIGGER chat_description_update
    BEFORE UPDATE OF description ON chat
    FOR EACH ROW
BEGIN
    SELECT
        CASE
            WHEN (OLD.type IN ('direct', 'self', 'thread'))
                THEN RAISE(ABORT, 'direct or self chats cannot change description')

            WHEN OLD.type = 'group' AND NEW.description IS NULL
                THEN RAISE(ABORT, 'group chats must have a description')
            END;
END;

CREATE TRIGGER chat_expires_at_update
    BEFORE UPDATE OF expires_at ON chat
    FOR EACH ROW
    WHEN OLD.type = 'thread' AND NEW.expires_at IS NOT NULL
BEGIN
    SELECT CASE
               WHEN (SELECT expires_at FROM chat WHERE id = OLD.parent_id) IS NOT NULL
                   AND NEW.expires_at > (SELECT expires_at FROM chat WHERE id = OLD.parent_id)
                   THEN RAISE(ABORT, 'thread chat: expires_at must be <= parent expires_at when parent is expirable')
               END;
END;

CREATE TRIGGER chat_type_rules_insert
    BEFORE INSERT ON chat
    FOR EACH ROW
BEGIN
    SET NEW.status = 'active';

    -- DIRECT
    SELECT CASE
               WHEN NEW.type = 'direct' AND (
                        NEW.visibility          != 'secret' OR
                        NEW.post_policy         != 'all'    OR
                        NEW.moderation_policy   != 'none'   OR
                        NEW.name                IS NOT NULL OR
                        NEW.topic               IS NOT NULL OR
                        NEW.description         IS NOT NULL OR
                        NEW.parent_id           IS NOT NULL OR
                        NEW.expires_at          IS NOT NULL OR
                        NEW.threads_enabled     != FALSE
                   )
                   THEN RAISE(ABORT, 'direct: invalid combination (visibility=secret, post_policy=all, moderation=none; name/topic/description/parent_id/expires_at must be NULL; threads_enabled=FALSE)')
               END;

    -- SELF
    SELECT CASE
               WHEN NEW.type = 'self' AND (
                   NEW.visibility                   != 'secret'
                       OR NEW.post_policy           != 'owner'
                       OR NEW.moderation_policy     != 'none'
                       OR NEW.name                  IS NOT NULL
                       OR NEW.topic                 IS NOT NULL
                       OR NEW.description           IS NOT NULL
                       OR NEW.parent_id             IS NOT NULL
                       OR NEW.expires_at            IS NOT NULL
                       OR NEW.threads_enabled       != FALSE
                   )
                   THEN RAISE(ABORT, 'self: invalid combination (visibility=secret, post_policy=owner, name/topic/description/parent_id/expires_at must be NULL; FALSE)')
               END;

    -- GROUP
    SELECT CASE
               WHEN NEW.type = 'group' AND (
                   NEW.name                     IS NULL
                       OR NEW.topic             IS NULL
                       OR NEW.description       IS NULL
                       OR NEW.parent_id         IS NOT NULL
                   )
                   THEN RAISE(ABORT, 'group: invalid combination (name/topic/description required; parent_id must be NULL)')
               END;

    -- THREAD
    -- Parent must exist, be GROUP, and have threads_enabled=TRUE.
    SELECT CASE
               WHEN NEW.type = 'thread' AND (
                   NEW.name                 IS NOT NULL
                       OR NEW.topic         IS NOT NULL
                       OR NEW.description   IS NOT NULL
                       OR NEW.parent_id     IS NULL
                   )
                   THEN RAISE(ABORT, 'thread: name/topic/description must be NULL; parent_id required')
               END;

    -- Enforce thread parent properties
    SELECT CASE
               WHEN NEW.type = 'thread' AND (
                    (SELECT type, status, threads_enabled FROM chat WHERE id = NEW.parent_id) != ('group', 'active', TRUE)
               )
                   THEN RAISE(ABORT, 'thread: parent must be an existing group with threads_enabled=TRUE')
               END;

    -- Visibility inheritance bounds for thread
    SELECT CASE
               WHEN NEW.type = 'thread' AND (
                   (CASE (SELECT visibility FROM chat WHERE id = NEW.parent_id)
                        WHEN 'private' THEN (NEW.visibility IN ('private', 'secret'))
                        WHEN 'secret'  THEN NEW.visibility = 'secret'
                       END) != TRUE
                   )
                   THEN RAISE(ABORT, 'thread: visibility cannot exceed parent visibility')
               END;

    -- Post Policy inheritance bounds for thread
    SELECT CASE
                WHEN NEW.type = 'thread' AND (
                    (CASE (SELECT post_policy FROM chat WHERE id = NEW.parent_id)
                         WHEN 'all'      THEN NEW.post_policy = 'all'
                         WHEN 'admins'   THEN (NEW.post_policy IN ('all', 'admins'))
                         WHEN 'owner'    THEN (NEW.post_policy IN ('all', 'owner'))
                        END) != TRUE
                    )
                    THEN RAISE(ABORT, 'thread: post_policy cannot exceed parent post_policy')
               END;

    -- Moderation Policy inheritance bounds for thread
    SELECT CASE
               WHEN NEW.type = 'thread' AND (
                   (CASE (SELECT moderation_policy FROM chat WHERE id = NEW.parent_id)
                        WHEN 'flagged_review'   THEN NEW.moderation_policy = 'auto_delete'
                        WHEN 'auto_delete'      THEN NEW.moderation_policy = 'auto_delete'
                       END) != TRUE
                   )
                   THEN RAISE(ABORT, 'thread: moderation_policy cannot exceed parent moderation_policy')
               END;

    -- Expires_at <= parent.expires_at when parent has one
    SELECT CASE
               WHEN NEW.type = 'thread'
                   AND NEW.expires_at IS NOT NULL
                   AND (SELECT expires_at FROM chat WHERE id = NEW.parent_id) IS NOT NULL
                   AND NEW.expires_at > (SELECT expires_at FROM chat WHERE id = NEW.parent_id)
                   THEN RAISE(ABORT, 'thread: expires_at must be <= parent expires_at when parent is expirable')
               END;

    -- Non-group types must not enable threads
    SELECT CASE
               WHEN NEW.type != 'group' AND NEW.threads_enabled = TRUE
                   THEN RAISE(ABORT, 'threads_enabled can only be true for group chats')
               END;
END;

-- ============================================================
-- 4) Parent → child cascading defaults for threads
--    * Visibility: tighten or broaden with care
--    * Status: archived dominates; locked forbids active; active re-inherits only if child had old value
--    * Expires: cap children at parent's expires_at
-- ============================================================

-- Visibility cascade:
-- If parent visibility tightens to 'secret' → force all child threads to 'secret'.
-- If parent changes to 'private' → any 'public' child becomes 'private' (secret stays secret).
-- If parent changes to 'public' → only children that were equal to OLD.visibility re-inherit to NEW.visibility.
CREATE TRIGGER chat_cascade_visibility
    AFTER UPDATE OF visibility ON chat
    FOR EACH ROW
BEGIN
    UPDATE chat
    SET visibility = CASE
            -- Condition 1: Parent -> secret: force all child threads to secret
            WHEN NEW.visibility = 'secret' THEN 'secret'

            -- Condition 2: Parent -> private: downgrade public children to private
            WHEN NEW.visibility = 'private' AND OLD.visibility != 'private' AND visibility = 'public' THEN 'private'

            -- Condition 3: Parent -> public (or any change): re-inherit only those that matched OLD
            WHEN NEW.visibility != OLD.visibility AND visibility = OLD.visibility THEN NEW.visibility

            -- Default case: do nothing
            ELSE visibility
        END
    WHERE parent_id = NEW.id;
END;

-- Post Policy cascade:
-- If parent post policy changes -> child changes to 'all'
CREATE TRIGGER chat_cascade_post_policy
    AFTER UPDATE OF post_policy ON chat
    FOR EACH ROW
BEGIN
    UPDATE chat
    SET post_policy = 'all'
    WHERE parent_id = NEW.id;
END;

-- Status cascade:
-- Parent -> archived: force all child threads to archived.
-- Parent -> locked: change active children to locked (archived stays archived).
-- Parent -> active: re-inherit only children that matched OLD.status.
CREATE TRIGGER chat_cascade_status
    AFTER UPDATE OF status ON chat
    FOR EACH ROW
BEGIN
    UPDATE chat
    SET status = CASE
        -- Any -> archived: Force all children to be archived
                     WHEN NEW.status = 'archived' THEN 'archived'
        -- Any -> locked: Upgrade active children to locked
                     WHEN NEW.status = 'locked' AND status = 'active' THEN 'locked'
        -- Re-inherit: Apply the new status to children that had the old status
                     WHEN status = OLD.status THEN NEW.status
        -- Default: If no condition is met, keep the current status
                     ELSE status
        END
    WHERE parent_id = NEW.id;
END;

-- Moderation Policy cascade:
-- Parent -> none: inherit children OLD.moderation_policy.
-- Parent -> flagged_review: change children to auto_delete.
-- Parent -> auto_delete: change children to auto_delete.
CREATE TRIGGER chat_cascade_moderation_policy
    AFTER UPDATE OF moderation_policy ON chat
    FOR EACH ROW
BEGIN
    UPDATE chat
    SET moderation_policy = CASE
                     WHEN NEW.moderation_policy = 'flagged_review' THEN 'auto_delete'
                     WHEN NEW.moderation_policy = 'auto_delete' THEN 'auto_delete'
                     ELSE moderation_policy
        END
    WHERE parent_id = NEW.id;
END;

-- Expires cascade:
-- Cap children at parent's expires_at when parent becomes/changes expirable.
CREATE TRIGGER chat_cascade_expires
    AFTER UPDATE OF expires_at ON chat
    FOR EACH ROW
    WHEN NEW.expires_at IS NOT NULL
BEGIN
    UPDATE chat
    SET expires_at = NEW.expires_at
    WHERE parent_id=NEW.id AND (expires_at IS NULL OR expires_at > NEW.expires_at);
END;

-- Chat member

CREATE TRIGGER chat_member_join_after_creation
    BEFORE INSERT ON chat_member
    FOR EACH ROW
    WHEN NEW.joined_at < (SELECT created_at FROM chat WHERE id = NEW.chat_id)
BEGIN
    SELECT RAISE(ABORT, 'joined_at cannot be before chat creation');
END;
CREATE TRIGGER chat_member_update_joined_at
    BEFORE UPDATE OF joined_at ON chat_member
    FOR EACH ROW
    WHEN NEW.joined_at < (SELECT created_at FROM chat WHERE id = NEW.chat_id)
BEGIN
    SELECT RAISE(ABORT, 'joined_at cannot be before chat creation');
END;

CREATE TRIGGER read_receipt_after_message
    BEFORE INSERT ON read_receipt
    FOR EACH ROW
    WHEN NEW.read_at < (SELECT sent_at FROM message WHERE id = NEW.message_id)
BEGIN
    SELECT RAISE(ABORT, 'read_at cannot be before message sent_at');
END;
CREATE TRIGGER read_receipt_update_read_at
    BEFORE UPDATE OF read_at ON read_receipt
    FOR EACH ROW
    WHEN NEW.read_at < (SELECT sent_at FROM message WHERE id = NEW.message_id)
BEGIN
    SELECT RAISE(ABORT, 'read_at cannot be before message sent_at');
END;

