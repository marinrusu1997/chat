-- This script will be executed ONCE on the master server's first startup
-- to create the initial database schema and extensions.
-- Patroni handles the creation of the replication user.

CREATE SCHEMA IF NOT EXISTS partman;

-- IMPORTANT: Extensions that affect schema and data (hstore, postgis, pg_partman)
-- must be created on the master. They will be replicated to the slaves.
CREATE EXTENSION IF NOT EXISTS hstore;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_partman WITH SCHEMA partman;
-- CREATE EXTENSION IF NOT EXISTS pg_cron;

-- Set the search path so that PostgreSQL can find the functions for your extensions.
SET search_path = public, partman;

-- Use \set to define a variable. Note: no semicolon is needed.
\set CLOCK_DRIFT '5 seconds'

\set GUEST_DEFAULT_PERMISSIONS_BITMASK B'0000000000000000000000000000000000000000000000000000000000000000'
\set BOT_DEFAULT_PERMISSIONS_BITMASK B'0000000000000000000000000000000000000000000000000000000111111111'
\set MEMBER_DEFAULT_PERMISSIONS_BITMASK B'0000000000000000000000000000000000000000000000000000000111111111'
\set MODERATOR_DEFAULT_PERMISSIONS_BITMASK B'0000000000000000000000000000000000000000000000000011111111111111'
\set ADMIN_DEFAULT_PERMISSIONS_BITMASK B'0000000000000000000000000000000000000000000000001111111111111111'
\set OWNER_DEFAULT_PERMISSIONS_BITMASK B'1111111111111111111111111111111111111111111111111111111111111111'

-- Utils
CREATE OR REPLACE FUNCTION check_whitelisted_updates(
    old_row HSTORE,
    new_row HSTORE,
    whitelist_columns TEXT[]
)
    RETURNS BOOLEAN AS $$
DECLARE
    changed_keys TEXT[];
    key TEXT;
    is_whitelisted BOOLEAN;
BEGIN
    -- Get a list of all keys where the value has changed
    WITH old_values AS (
        SELECT (EACH(old_row)).key AS k, (EACH(old_row)).value AS v
    ),
     new_values AS (
         SELECT (EACH(new_row)).key AS k, (EACH(new_row)).value AS v
     )
    SELECT ARRAY_AGG(COALESCE(old_values.k, new_values.k)) AS changed_keys
    FROM old_values FULL OUTER JOIN new_values ON old_values.k = new_values.k
    WHERE old_values.v IS DISTINCT FROM new_values.v;

    -- Iterate through the changed keys
    FOREACH key IN ARRAY changed_keys
        LOOP
            is_whitelisted := key = ANY(whitelist_columns);

            -- If a changed key is NOT in the whitelist, raise an exception
            IF NOT is_whitelisted THEN
                RAISE EXCEPTION 'Column "%" cannot be updated.', key;
                RETURN FALSE; -- This is unreachable, but good practice
            END IF;
        END LOOP;

    RETURN TRUE; -- All changes are whitelisted
END;
$$ LANGUAGE plpgsql;

-- User table
CREATE TABLE IF NOT EXISTS "user"
(
    id                      INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- credentials
    email                   VARCHAR(255) NOT NULL UNIQUE CHECK (LENGTH(email) >= 5 AND email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$'),
    password_hash           VARCHAR(255) NOT NULL CHECK (LENGTH(password_hash) >= 50),
    password_algo           SMALLINT NOT NULL CHECK (password_algo BETWEEN 0 AND 100),
    password_updated_at     TIMESTAMPTZ CHECK (password_updated_at IS NULL OR (password_updated_at >= COALESCE(last_active_at, created_at) AND password_updated_at <= NOW() + INTERVAL :'CLOCK_DRIFT')),
    -- PII
    name                    VARCHAR(255) NOT NULL CHECK (LENGTH(name) >= 2),
    -- activity
    last_login_at           TIMESTAMPTZ CHECK (last_login_at IS NULL OR (last_login_at > created_at AND last_login_at <= NOW() + INTERVAL :'CLOCK_DRIFT')),
    last_active_at          TIMESTAMPTZ CHECK (last_active_at IS NULL OR (last_active_at >= COALESCE(last_login_at, created_at) AND last_active_at <= NOW() + INTERVAL :'CLOCK_DRIFT')),
    created_at              TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL :'CLOCK_DRIFT') DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS session
(
    id                  BIGSERIAL,
    user_id             INTEGER NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    device              JSONB CHECK (
                        jsonb_typeof(device #> '{client, model}') = 'string' AND
                        jsonb_typeof(device #> '{client, version}') = 'string' AND
                        jsonb_typeof(device #> '{os, name}') = 'string' AND
                        jsonb_typeof(device #> '{os, version}') = 'string' AND

                        device #>> '{client, type}' IN ('browser', 'mobile', 'desktop') AND
                        LENGTH(device #>> '{client, model}') > 0 AND LENGTH(device #>> '{client, model}') <= 100 AND
                        LENGTH(device #>> '{client, version}') > 0 AND LENGTH(device #>> '{client, version}') <= 50 AND

                        LENGTH(device #>> '{os, name}') > 0 AND LENGTH(device #>> '{os, name}') <= 100 AND
                        LENGTH(device #>> '{os, version}') > 0 AND LENGTH(device #>> '{os, version}') <= 50 AND
                        device #>> '{os, platform}' IN ('ARM', 'x64', 'x86', 'MIPS')
    ),
    refresh_token_hash  BYTEA NOT NULL CHECK (OCTET_LENGTH(refresh_token_hash) = 32),
    refresh_count       SMALLINT NOT NULL CHECK (refresh_count <= 5000) DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL :'CLOCK_DRIFT') DEFAULT NOW(),
    expires_at          TIMESTAMPTZ NOT NULL CHECK (expires_at > created_at + INTERVAL '1 day'),
    ip                  INET NOT NULL,
    geo                 GEOGRAPHY(POINT) NOT NULL,
    user_agent          TEXT CHECK (LENGTH(user_agent) > 0),

    PRIMARY KEY (id, expires_at)
) PARTITION BY RANGE (expires_at);

--
-- Create a TEMPLATE table for `pg_partman` to use.
-- This table defines the storage parameters for all child partitions.
--
CREATE TABLE session_template (LIKE session);
ALTER TABLE session_template SET (
    autovacuum_vacuum_scale_factor = 0.0,
    autovacuum_vacuum_threshold = 100000,
    autovacuum_analyze_scale_factor = 0.05,
    autovacuum_analyze_threshold = 50
);

SELECT partman.create_parent(
   p_parent_table   := 'public.session',
   p_control        := 'expires_at',
   p_type           := 'range',
   p_interval       := '1 day',
   p_premake        := 7,
   p_template_table := 'public.session_template'
);

UPDATE partman.part_config
SET
    retention = '30 days',
    retention_keep_table = false,
    retention_keep_index = false
WHERE parent_table = 'public.session';

-- SELECT cron.schedule(
--     'session_partitions_maintenance',
--     '0 * * * *', -- every hour
--     $$ SELECT partman.run_maintenance(); $$
-- );

-- Chat table
CREATE TYPE chat_type_enum AS ENUM ('direct', 'group', 'self', 'thread');
CREATE TYPE chat_visibility_enum AS ENUM ('public', 'private', 'secret');
CREATE TYPE chat_post_policy_enum AS ENUM ('all', 'admins', 'owner');
CREATE TYPE chat_status_enum AS ENUM ('active', 'archived', 'locked');
CREATE TYPE chat_moderation_policy_enum AS ENUM ('none', 'flagged_review', 'auto_delete');

CREATE TABLE IF NOT EXISTS chat
(
    id                  INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- enums
    type                chat_type_enum NOT NULL,
    visibility          chat_visibility_enum NOT NULL,
    post_policy         chat_post_policy_enum NOT NULL,
    status              chat_status_enum NOT NULL DEFAULT 'active',
    moderation_policy   chat_moderation_policy_enum NOT NULL DEFAULT 'none',
    -- settings / presentation
    name                VARCHAR(100) CHECK (name IS NULL OR LENGTH(name) >= 2),
    name_fts            tsvector GENERATED ALWAYS AS (
                            CASE
                                WHEN type = 'group' AND visibility = 'public' AND name IS NOT NULL
                                    THEN to_tsvector('english', name)
                            END
                        ) STORED,
    tags                TEXT[],
    topic               VARCHAR(100) CHECK (topic IS NULL OR LENGTH(topic) >= 2),
    description         VARCHAR(500) CHECK (description IS NULL OR LENGTH(description) >= 2),
    settings            JSONB NOT NULL DEFAULT '{}',
    -- authorship / lineage
    created_by          INTEGER NOT NULL REFERENCES "user"(id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL :'CLOCK_DRIFT') DEFAULT NOW(),
    parent_id           INTEGER REFERENCES chat(id) ON DELETE CASCADE,
    expires_at          TIMESTAMPTZ CHECK (expires_at IS NULL OR (expires_at > NOW())),
    -- group thread toggle (global per chat)
    threads_enabled     BOOLEAN NOT NULL DEFAULT FALSE, -- @fixme needs to also be set per message

    CHECK (type NOT IN ('direct', 'self', 'thread') OR ((name, tags, topic, description) IS NOT DISTINCT FROM (NULL, NULL, NULL, NULL))),
    CHECK (type IS DISTINCT FROM 'group' OR (name IS NOT NULL AND tags IS NOT NULL AND topic IS NOT NULL AND description IS NOT NULL)),

    CHECK (type NOT IN ('direct', 'self') OR expires_at IS NULL),

    CHECK (type NOT IN ('direct', 'self') OR (
        visibility = 'secret' AND
        post_policy = 'owner' AND
        moderation_policy = 'none' AND
        status != 'archived'
    ))
) PARTITION BY HASH (id);

CREATE TABLE chat_0 PARTITION OF chat
    FOR VALUES WITH (MODULUS 8, REMAINDER 0);

CREATE TABLE chat_1 PARTITION OF chat
    FOR VALUES WITH (MODULUS 8, REMAINDER 1);

CREATE TABLE chat_2 PARTITION OF chat
    FOR VALUES WITH (MODULUS 8, REMAINDER 2);

CREATE TABLE chat_3 PARTITION OF chat
    FOR VALUES WITH (MODULUS 8, REMAINDER 3);

CREATE TABLE chat_4 PARTITION OF chat
    FOR VALUES WITH (MODULUS 8, REMAINDER 4);

CREATE TABLE chat_5 PARTITION OF chat
    FOR VALUES WITH (MODULUS 8, REMAINDER 5);

CREATE TABLE chat_6 PARTITION OF chat
    FOR VALUES WITH (MODULUS 8, REMAINDER 6);

CREATE TABLE chat_7 PARTITION OF chat
    FOR VALUES WITH (MODULUS 8, REMAINDER 7);

-- User Participant junction table
CREATE TYPE chat_participant_role_enum AS ENUM ('owner', 'admin', 'moderator', 'member', 'guest', 'bot');
CREATE TYPE chat_participant_ban_reason_enum AS ENUM ('spam', 'abuse', 'harassment', 'scam', 'policy_violation', 'other');
CREATE TYPE chat_participant_ban_type_enum AS ENUM ('temporary', 'permanent', 'shadow');
CREATE TYPE chat_participant_notification_level_enum AS ENUM ('all', 'mentions_only', 'important_only', 'none');

CREATE TABLE IF NOT EXISTS chat_participant (
    -- identifiers & relations
    chat_id                 INTEGER NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id                 INTEGER NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    chat_type               chat_type_enum NOT NULL,
    -- membership & roles
    role                    chat_participant_role_enum NOT NULL,
    permissions_bitmask     BIT(64) NOT NULL,
    -- lifecycle
    joined_at               TIMESTAMPTZ NOT NULL CHECK (joined_at <= NOW() + INTERVAL :'CLOCK_DRIFT') DEFAULT NOW(),
    rejoined_at             TIMESTAMPTZ CHECK (rejoined_at IS NULL OR (rejoined_at > joined_at AND rejoined_at <= NOW() + INTERVAL :'CLOCK_DRIFT')),
    left_at                 TIMESTAMPTZ CHECK (left_at IS NULL OR (left_at > joined_at AND left_at <= NOW() + INTERVAL :'CLOCK_DRIFT')),
    -- moderation
    ban_reason              chat_participant_ban_reason_enum,
    ban_type                chat_participant_ban_type_enum,
    banned_by               INTEGER REFERENCES "user"(id) ON DELETE RESTRICT,
    banned_until            TIMESTAMPTZ CHECK (banned_until IS NULL OR (banned_until > NOW())),
    ban_reason_note         VARCHAR(200) CHECK (LENGTH(ban_reason_note) >= 1),
    -- invitations
    invited_by              INTEGER REFERENCES "user"(id) ON DELETE RESTRICT,
    invited_at              TIMESTAMPTZ CHECK (invited_at IS NULL OR (invited_at < joined_at)),
    -- read tracking & activity
    last_read_message_id    UUID,
    last_read_at            TIMESTAMPTZ CHECK (last_read_at IS NULL OR (last_read_at > joined_at AND last_read_at <= NOW() + INTERVAL :'CLOCK_DRIFT')),
    -- presence_status      TEXT NOT NULL CHECK (presence_status IN ('online', 'offline', 'idle', 'dnd')) DEFAULT 'offline', @fixme will be stored in Redis
    -- notifications & preferences
    muted_until             TIMESTAMPTZ CHECK (muted_until IS NULL OR (muted_until > NOW() + INTERVAL :'CLOCK_DRIFT')),
    notification_level      chat_participant_notification_level_enum NOT NULL DEFAULT 'all',
    custom_nickname         VARCHAR(100) CHECK (LENGTH(custom_nickname) >= 1),
    color_theme             VARCHAR(50) CHECK (LENGTH(color_theme) >= 1),
    settings                JSONB NOT NULL DEFAULT '{}',
    -- pinning & tagging
    is_pinned               BOOLEAN NOT NULL DEFAULT FALSE,
    last_pinned_message_id  UUID,

    PRIMARY KEY (user_id, chat_id), -- to find all chats of user

    CHECK (
        (role = 'guest' AND permissions_bitmask = :'GUEST_DEFAULT_PERMISSIONS_BITMASK') OR
        (role = 'bot' AND (permissions_bitmask & :'BOT_DEFAULT_PERMISSIONS_BITMASK') = permissions_bitmask) OR
        (role = 'member' AND (permissions_bitmask & :'MEMBER_DEFAULT_PERMISSIONS_BITMASK') = permissions_bitmask) OR
        (role = 'moderator' AND (permissions_bitmask & :'MODERATOR_DEFAULT_PERMISSIONS_BITMASK') = permissions_bitmask) OR
        (role = 'admin' AND (permissions_bitmask & :'ADMIN_DEFAULT_PERMISSIONS_BITMASK') = permissions_bitmask) OR
        (role = 'owner' AND permissions_bitmask = :'OWNER_DEFAULT_PERMISSIONS_BITMASK')
    ),

    CHECK (role IS DISTINCT FROM 'bot' OR (
            color_theme             IS NULL AND
            last_pinned_message_id  IS NULL
          )
    ),
    CHECK (role IS DISTINCT FROM 'guest' OR (
            rejoined_at IS NULL AND
            left_at     IS NULL
          )
    ),
    CHECK (role IS DISTINCT FROM 'owner' OR (
                rejoined_at         IS NULL AND
                left_at             IS NULL AND
                ban_reason  IS NULL AND
                ban_reason_note  IS NULL AND
                ban_type            IS NULL AND
                banned_until        IS NULL AND
                banned_by           IS NULL AND
                invited_by          IS NULL AND
                invited_at          IS NULL
          )
    ),

    CHECK (
        (rejoined_at IS NULL AND left_at IS NULL) OR
        (rejoined_at IS NULL AND left_at IS NOT NULL) OR
        (rejoined_at IS NOT NULL AND left_at IS NULL)
    ),

    CHECK (
        (
            ban_reason      IS NULL AND
            ban_type        IS NULL AND
            banned_by       IS NULL AND
            banned_until    IS NULL AND
            ban_reason_note IS NULL
        ) OR
        (
            ban_reason      IS NOT NULL AND
            ban_type        IS NOT NULL AND
            banned_by       IS NOT NULL
        )
    ),
    CHECK (ban_type IS DISTINCT FROM 'temporary' OR banned_until IS NOT NULL),
    CHECK (ban_type IS DISTINCT FROM 'permanent' OR banned_until IS NULL),

    CHECK (
        (invited_at IS NULL AND invited_by IS NULL) OR
        (invited_at IS NOT NULL AND invited_by IS NOT NULL)
    ),

    CHECK (banned_by != user_id AND invited_by != user_id),

    CHECK (
        (last_read_message_id IS NULL AND last_read_at IS NULL) OR
        (last_read_message_id IS NOT NULL AND last_read_at IS NOT NULL)
    )
) PARTITION BY HASH (chat_id);

CREATE TABLE chat_participant_0 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 0);

CREATE TABLE chat_participant_1 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 1);

CREATE TABLE chat_participant_2 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 2);

CREATE TABLE chat_participant_3 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 3);

CREATE TABLE chat_participant_4 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 4);

CREATE TABLE chat_participant_5 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 5);

CREATE TABLE chat_participant_6 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 6);

CREATE TABLE chat_participant_7 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 7);

CREATE TABLE chat_participant_8 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 8);

CREATE TABLE chat_participant_9 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 9);

CREATE TABLE chat_participant_10 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 10);

CREATE TABLE chat_participant_11 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 11);

CREATE TABLE chat_participant_12 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 12);

CREATE TABLE chat_participant_13 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 13);

CREATE TABLE chat_participant_14 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 14);

CREATE TABLE chat_participant_15 PARTITION OF chat_participant
    FOR VALUES WITH (MODULUS 16, REMAINDER 15);

DO $$
    DECLARE
        r RECORD;
    BEGIN
        FOR r IN (
            SELECT
                c.relname AS child_table
            FROM
                pg_inherits AS i
                    JOIN pg_class AS p ON p.oid = i.inhparent
                    JOIN pg_class AS c ON c.oid = i.inhrelid
                    JOIN pg_namespace AS n ON n.oid = p.relnamespace
            WHERE
                p.relname = 'chat_participant' AND n.nspname = 'public'
        )
            LOOP
                EXECUTE format('ALTER TABLE public.%I SET (autovacuum_vacuum_scale_factor = 0.05, autovacuum_vacuum_threshold = 50, autovacuum_analyze_scale_factor = 0.05, autovacuum_analyze_threshold = 50)', r.child_table);
            END LOOP;
    END
$$;

-- Indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_email                    ON "user"(email) INCLUDE (password_hash, password_algo);

CREATE INDEX IF NOT EXISTS idx_session_user_id                      ON session(user_id);
CREATE INDEX IF NOT EXISTS idx_session_expires_at                   ON session USING BRIN(expires_at) WITH (pages_per_range = 64);
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_refresh_token_hash    ON session(refresh_token_hash, expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_user_created_at       ON session(user_id, created_at, expires_at);

CREATE INDEX IF NOT EXISTS idx_chat_name_fts                        ON chat USING GIN(name_fts) WHERE name_fts IS NOT NULL AND type = 'group' AND visibility = 'public';
CREATE INDEX IF NOT EXISTS idx_chat_tags                            ON chat USING GIN(tags) WHERE tags IS NOT NULL AND type = 'group' AND visibility = 'public';
CREATE INDEX IF NOT EXISTS idx_chat_parent                          ON chat USING BTREE(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_chat_status                          ON chat USING HASH(status) WHERE status IN ('active', 'locked');

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_participant_single_owner ON chat_participant(chat_id) WHERE role = 'owner' AND chat_type <> 'direct';
CREATE INDEX IF NOT EXISTS idx_chat_participant_chat_id             ON chat_participant(chat_id, user_id); -- to find all members of chat
CREATE INDEX IF NOT EXISTS idx_chat_participant_role                ON chat_participant(chat_id, role) INCLUDE (user_id);
CREATE INDEX IF NOT EXISTS idx_chat_participant_left_at             ON chat_participant(chat_id, left_at) INCLUDE (user_id) WHERE left_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_chat_participant_ban                 ON chat_participant(chat_id, ban_reason) INCLUDE (user_id) WHERE ban_reason IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_chat_participant_last_read           ON chat_participant(chat_id, muted_until) INCLUDE (user_id) WHERE muted_until IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_chat_participant_notification_level  ON chat_participant(chat_id, notification_level) INCLUDE (user_id) WHERE notification_level <> 'none';

-- Triggers
--- User
CREATE OR REPLACE FUNCTION user_immutable_columns()
    RETURNS TRIGGER AS $$
DECLARE
    changed_cols text := '';
BEGIN
    IF NEW.email IS DISTINCT FROM OLD.email THEN
        changed_cols := changed_cols || ' email';
    END IF;
    IF NEW.name IS DISTINCT FROM OLD.name THEN
        changed_cols := changed_cols || ' name';
    END IF;
    IF NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        changed_cols := changed_cols || ' created_at';
    END IF;

    IF changed_cols <> '' THEN
        RAISE EXCEPTION 'Immutable columns of user % changed: %', OLD.id, changed_cols;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER user_immutable_columns_trigger
    BEFORE UPDATE OF email, name, created_at ON "user"
    FOR EACH ROW
EXECUTE FUNCTION user_immutable_columns();

CREATE OR REPLACE FUNCTION user_password_insert()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.password_updated_at IS NOT NULL THEN
        RAISE EXCEPTION 'password_updated_at of user % must be NULL on creation', NEW.id;
    END IF;

    RETURN NEW;  -- Must return NEW for BEFORE INSERT triggers
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER user_password_insert_trigger
    BEFORE INSERT ON "user"
    FOR EACH ROW
EXECUTE FUNCTION user_password_insert();

CREATE OR REPLACE FUNCTION user_password_update()
    RETURNS TRIGGER AS $$
BEGIN
    -- Validate password changes
    IF NEW.password_hash = OLD.password_hash AND NEW.password_algo != OLD.password_algo THEN
        RAISE EXCEPTION 'password_hash of user % must also change when password_algo changes', OLD.id;
    ELSIF NEW.password_hash = OLD.password_hash THEN
        RAISE EXCEPTION 'password_hash of user % must be different from old value', OLD.id;
    ELSIF NEW.password_algo <= OLD.password_algo THEN
        RAISE EXCEPTION 'password_algo of user % must increase, old=% and new=%', OLD.id, OLD.password_algo, NEW.password_algo;
    END IF;

    -- Update the timestamp
    NEW.password_updated_at := CURRENT_TIMESTAMP;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER user_password_update_trigger
    BEFORE UPDATE OF password_hash, password_algo ON "user"
    FOR EACH ROW
EXECUTE FUNCTION user_password_update();

CREATE OR REPLACE FUNCTION validations_before_user_deletion()
    RETURNS TRIGGER AS $$
BEGIN
    UPDATE chat_participant AS cp
    SET
        invited_by = CASE WHEN cp.invited_by = OLD.id THEN owner.user_id ELSE cp.invited_by END,
        banned_by  = CASE WHEN cp.banned_by  = OLD.id THEN owner.user_id ELSE cp.banned_by  END
    FROM chat_participant AS owner
    WHERE
        cp.chat_id = owner.chat_id AND owner.role = 'owner'
        AND (cp.invited_by = OLD.id OR cp.banned_by = OLD.id)
        AND NOT EXISTS (
            SELECT 1
            FROM chat_participant
            WHERE user_id = OLD.id AND chat_id = cp.chat_id AND role = 'owner'
        );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER validations_before_user_deletion_trigger
    BEFORE DELETE ON "user"
    FOR EACH ROW
EXECUTE FUNCTION validations_before_user_deletion();

-- Session
CREATE OR REPLACE FUNCTION enforce_max_sessions()
    RETURNS TRIGGER AS $$
DECLARE
    session_count INT;
BEGIN
    -- Lock all sessions for this user across partitions
    PERFORM 1 FROM session WHERE user_id = NEW.user_id FOR UPDATE;

    -- Count how many active sessions user already has
    SELECT COUNT(*) INTO STRICT session_count FROM session WHERE user_id = NEW.user_id;

    IF session_count >= 10 THEN
        -- Delete oldest sessions using the composite index
        WITH sessions_to_delete AS (
            SELECT id
            FROM session
            WHERE user_id = NEW.user_id
            ORDER BY created_at ASC
            LIMIT (session_count - 9)
        )
        DELETE FROM session
        USING sessions_to_delete
        WHERE session.id = sessions_to_delete.id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER sessions_limit_trigger
    BEFORE INSERT ON session
    FOR EACH ROW
EXECUTE FUNCTION enforce_max_sessions();

CREATE OR REPLACE FUNCTION prevent_modification_of_session()
    RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Cannot modify session % of user %', OLD.id, OLD.user_id;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER enforce_readonly_session_row
    BEFORE UPDATE ON session
    FOR EACH ROW
EXECUTE FUNCTION prevent_modification_of_session();

--- Chat
-- ============================================================
-- 1) Read-only when ARCHIVED
--    If a row is archived, it cannot be updated anymore.
-- ============================================================
CREATE OR REPLACE FUNCTION chat_readonly_if_archived()
    RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status = 'archived' THEN
        RAISE EXCEPTION '% chat(%) is archived and read-only', OLD.type, OLD.id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_readonly_if_archived_trigger
    BEFORE UPDATE ON chat
    FOR EACH ROW
EXECUTE FUNCTION chat_readonly_if_archived();

-- ============================================================
-- 2) Immutable columns: type, name, created_by, created_at, parent_id, threads_enabled
--    Build a single message listing all changed immutables.
-- ============================================================
CREATE OR REPLACE FUNCTION chat_immutables_guard()
    RETURNS TRIGGER AS $$
DECLARE
    changed_cols text := '';
BEGIN
    IF NEW.type IS DISTINCT FROM OLD.type THEN
        changed_cols := changed_cols || ' type';
    END IF;
    IF NEW.name IS DISTINCT FROM OLD.name THEN
        changed_cols := changed_cols || ' name';
    END IF;
    IF NEW.created_by IS DISTINCT FROM OLD.created_by THEN
        changed_cols := changed_cols || ' created_by';
    END IF;
    IF NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        changed_cols := changed_cols || ' created_at';
    END IF;
    IF NEW.parent_id IS DISTINCT FROM OLD.parent_id THEN
        changed_cols := changed_cols || ' parent_id';
    END IF;
    IF NEW.threads_enabled IS DISTINCT FROM OLD.threads_enabled THEN
        changed_cols := changed_cols || ' threads_enabled';
    END IF;

    IF changed_cols <> '' THEN
        RAISE EXCEPTION 'Immutable columns of % chat(%) changed: %', OLD.type, OLD.id, changed_cols;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_immutables_guard_trigger
    BEFORE UPDATE OF type, name, created_by, created_at, parent_id, threads_enabled ON chat
    FOR EACH ROW
EXECUTE FUNCTION chat_immutables_guard();

-- ============================================================
-- 3) Type-specific constraints on UPDATE
--    Parent-related checks done here for threads.
-- ============================================================
CREATE TYPE parent_chat AS (
    visibility          chat_visibility_enum,
    post_policy         chat_post_policy_enum,
    status              chat_status_enum,
    moderation_policy   chat_moderation_policy_enum,
    created_by          INTEGER,
    expires_at          TIMESTAMPTZ
);

CREATE OR REPLACE FUNCTION chat_parent_constraints()
    RETURNS TRIGGER AS $$
DECLARE
    parent_row parent_chat;
BEGIN
    SELECT visibility, post_policy, status, moderation_policy, created_by, expires_at
    INTO STRICT parent_row
    FROM chat
    WHERE id = OLD.parent_id;

    -- === Visibility rules ===
    IF parent_row.visibility = 'private' AND NOT (NEW.visibility IN ('private', 'secret')) THEN
        RAISE EXCEPTION 'thread (%): visibility "%" cannot exceed parent (%) visibility "%"',
            NEW.id, NEW.visibility, OLD.parent_id, parent_row.visibility;
    ELSIF parent_row.visibility = 'secret' AND NEW.visibility != 'secret' THEN
        RAISE EXCEPTION 'thread (%): visibility "%" cannot exceed parent (%) visibility "%"',
            NEW.id, NEW.visibility, OLD.parent_id, parent_row.visibility;
    END IF;

    -- === Post policy rules ===
    IF parent_row.post_policy = 'all' AND NEW.post_policy != 'all' THEN
        RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
            NEW.id, NEW.post_policy, OLD.parent_id, parent_row.post_policy;
    ELSIF parent_row.post_policy = 'admins' AND NOT (NEW.post_policy IN ('all','admins')) THEN
        RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
            NEW.id, NEW.post_policy, OLD.parent_id, parent_row.post_policy;
    ELSIF parent_row.post_policy = 'owner' AND NOT (NEW.post_policy IN ('all','owner')) THEN
        RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
            NEW.id, NEW.post_policy, OLD.parent_id, parent_row.post_policy;
    END IF;

    -- === Status rules ===
    IF parent_row.status = 'locked' AND NOT (NEW.status IN ('archived','locked')) THEN
        RAISE EXCEPTION 'thread (%): status "%" cannot exceed parent (%) status "%"',
            NEW.id, NEW.status, OLD.parent_id, parent_row.status;
    END IF;

    -- === Moderation policy rules ===
    IF parent_row.moderation_policy IN ('flagged_review','auto_delete') AND NEW.moderation_policy != 'auto_delete' THEN
        RAISE EXCEPTION 'thread (%): moderation_policy "%" cannot exceed parent (%) moderation_policy "%"',
            NEW.id, NEW.moderation_policy, OLD.parent_id, parent_row.moderation_policy;
    END IF;

    -- === Expiration rules ===
    IF parent_row.expires_at IS NOT NULL AND (NEW.expires_at IS NULL OR NEW.expires_at > parent_row.expires_at) THEN
        NEW.expires_at = parent_row.expires_at;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_parent_constraints_trigger
    BEFORE UPDATE OF visibility, post_policy, status, moderation_policy, expires_at ON chat
    FOR EACH ROW
    WHEN (OLD.type = 'thread')
EXECUTE FUNCTION chat_parent_constraints();

CREATE OR REPLACE FUNCTION chat_can_have_threads(r_chat chat)
    RETURNS BOOLEAN as $$
BEGIN
    RETURN r_chat.type = 'group' AND r_chat.status = 'active' AND r_chat.threads_enabled = TRUE;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION chat_type_rules_insert()
    RETURNS TRIGGER AS $$
DECLARE
    parent_row parent_chat;
BEGIN
    -- Default status
    NEW.status := 'active';

    -- Non-group chats cannot enable threads
    IF NEW.type != 'group' AND NEW.threads_enabled IS TRUE THEN
        RAISE EXCEPTION '% chat (%) cannot have threads_enabled', NEW.type, NEW.id;
    END IF;

    IF NEW.type == 'thread' THEN
        IF NEW.parent_id IS NULL THEN
            RAISE EXCEPTION 'thread (%): parent_id is required', NEW.id;
        END IF;

        SELECT visibility, post_policy, status, moderation_policy, created_by, expires_at
        INTO STRICT parent_row
        FROM chat
        WHERE id = OLD.parent_id;

        IF chat_can_have_threads(parent_row) IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'thread (%): parent (%) must be an existing group with threads_enabled=TRUE',
                NEW.id, OLD.parent_id;
        END IF;

        -- Visibility bounds
        IF parent_row.visibility = 'private' AND NOT (NEW.visibility IN ('private', 'secret')) THEN
            RAISE EXCEPTION 'thread (%): visibility "%" cannot exceed parent (%) visibility "%"',
                NEW.id, NEW.visibility, OLD.parent_id, parent_row.visibility;
        ELSIF parent_row.visibility = 'secret' AND NEW.visibility != 'secret' THEN
            RAISE EXCEPTION 'thread (%): visibility "%" cannot exceed parent (%) visibility "%"',
                NEW.id, NEW.visibility, OLD.parent_id, parent_row.visibility;
        END IF;

        -- Post policy bounds
        IF parent_row.post_policy = 'all' AND NEW.post_policy != 'all' THEN
            RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
                NEW.id, NEW.post_policy, OLD.parent_id, parent_row.post_policy;
        ELSIF parent_row.post_policy = 'admins' AND NOT (NEW.post_policy IN ('all','admins')) THEN
            RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
                NEW.id, NEW.post_policy, OLD.parent_id, parent_row.post_policy;
        ELSIF parent_row.post_policy = 'owner' AND NOT (NEW.post_policy IN ('all','owner')) THEN
            RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
                NEW.id, NEW.post_policy, OLD.parent_id, parent_row.post_policy;
        END IF;

        -- Moderation policy bounds
        IF parent_row.moderation_policy IN ('flagged_review','auto_delete') AND NEW.moderation_policy != 'auto_delete' THEN
            RAISE EXCEPTION 'thread (%): moderation_policy "%" cannot exceed parent (%) moderation_policy "%"',
                NEW.id, NEW.moderation_policy, OLD.parent_id, parent_row.moderation_policy;
        END IF;

        -- Expires At bound
        IF parent_row.expires_at IS NOT NULL AND (NEW.expires_at IS NULL OR NEW.expires_at > parent_row.expires_at) THEN
            NEW.expires_at = parent_row.expires_at;
        END IF;

        NEW.created_by = parent_row.created_by;
    ELSIF NEW.parent_id IS NOT NULL THEN
        RAISE EXCEPTION '% chat (%) cannot have parent_id (%)', NEW.type, NEW.id, NEW.parent_id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_type_rules_insert_trigger
    BEFORE INSERT ON chat
    FOR EACH ROW
EXECUTE FUNCTION chat_type_rules_insert();

CREATE OR REPLACE FUNCTION chat_after_insert_add_owner()
    RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO chat_participant(chat_id, user_id, chat_type, role, permissions_bitmask)
    VALUES (
            NEW.id,
            NEW.created_by,
            NEW.type,
            'owner',
            B'1111111111111111111111111111111111111111111111111111111111111111'
    );

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_after_insert_add_owner_trigger
    AFTER INSERT ON chat
    FOR EACH ROW
EXECUTE FUNCTION chat_after_insert_add_owner();

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
CREATE OR REPLACE FUNCTION chat_cascade_visibility()
    RETURNS TRIGGER AS $$
BEGIN
    UPDATE chat
    SET visibility = CASE
            -- Condition 1: Parent -> secret: force all child threads to secret
            WHEN NEW.visibility = 'secret' THEN 'secret'
            -- Condition 2: Parent -> private: downgrade public children to private
            WHEN NEW.visibility = 'private' AND OLD.visibility != 'private' AND visibility = 'public' THEN 'private'
            -- Condition 3: Parent -> any other change: re-inherit only those that matched OLD
            WHEN NEW.visibility != OLD.visibility AND visibility = OLD.visibility THEN NEW.visibility
            -- Default: keep existing
            ELSE visibility
        END
    WHERE parent_id = OLD.id;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_cascade_visibility_trigger
    AFTER UPDATE OF visibility ON chat
    FOR EACH ROW
    WHEN (NEW.visibility IS DISTINCT FROM OLD.visibility AND chat_can_have_threads(OLD))
EXECUTE FUNCTION chat_cascade_visibility();


-- Post Policy cascade:
-- If parent post policy changes -> child changes to 'all'
CREATE OR REPLACE FUNCTION chat_cascade_post_policy()
    RETURNS TRIGGER AS $$
BEGIN
    UPDATE chat
    SET post_policy = 'all'
    WHERE parent_id = OLD.id AND post_policy IS DISTINCT FROM 'all';

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_cascade_post_policy_trigger
    AFTER UPDATE OF post_policy ON chat
    FOR EACH ROW
    WHEN (NEW.post_policy IS DISTINCT FROM OLD.post_policy AND chat_can_have_threads(OLD))
EXECUTE FUNCTION chat_cascade_post_policy();

-- Status cascade:
-- Parent -> archived: force all child threads to archived.
-- Parent -> locked: change active children to locked (archived stays archived).
-- Parent -> active: re-inherit only children that matched OLD.status.
CREATE OR REPLACE FUNCTION chat_cascade_status()
    RETURNS TRIGGER AS $$
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

    RETURN NULL; -- AFTER triggers must return NULL
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_cascade_status_trigger
    AFTER UPDATE OF status ON chat
    FOR EACH ROW
    WHEN (NEW.status IS DISTINCT FROM OLD.status AND chat_can_have_threads(OLD))
EXECUTE FUNCTION chat_cascade_status();

-- Moderation Policy cascade:
-- Parent -> none: inherit children OLD.moderation_policy.
-- Parent -> flagged_review: change children to auto_delete.
-- Parent -> auto_delete: change children to auto_delete.
CREATE OR REPLACE FUNCTION chat_cascade_moderation_policy()
    RETURNS TRIGGER AS $$
BEGIN
    UPDATE chat
    SET moderation_policy = 'auto_delete'
    WHERE parent_id = OLD.id;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_cascade_moderation_policy_trigger
    AFTER UPDATE OF moderation_policy ON chat
    FOR EACH ROW
    WHEN (NEW.moderation_policy IS DISTINCT FROM OLD.moderation_policy AND
          NEW.moderation_policy IN ('flagged_review', 'auto_delete') AND
          chat_can_have_threads(OLD)
        )
EXECUTE FUNCTION chat_cascade_moderation_policy();

-- Expires cascade:
-- Cap children at parent's expires_at when parent becomes/changes expirable.
CREATE OR REPLACE FUNCTION chat_cascade_expires()
    RETURNS TRIGGER AS $$
BEGIN
    UPDATE chat
    SET expires_at = NEW.expires_at
    WHERE parent_id = NEW.id AND (expires_at IS NULL OR expires_at > NEW.expires_at);

    RETURN NULL; -- AFTER triggers must return NULL
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_cascade_expires_trigger
    AFTER UPDATE OF expires_at ON chat
    FOR EACH ROW
    WHEN (NEW.expires_at IS NOT NULL AND
          NEW.expires_at IS DISTINCT FROM OLD.expires_at AND
          chat_can_have_threads(OLD)
        )
EXECUTE FUNCTION chat_cascade_expires();


-- Chat member
CREATE TYPE chat_inviter AS (
    role                    chat_participant_role_enum,
    permissions_bitmask     BIGINT
);
CREATE TYPE chat_where_user_joined AS (
    type            chat_type_enum,
    visibility      chat_visibility_enum,
    status          chat_status_enum,
    created_at      TIMESTAMPTZ,
    created_by      INTEGER
);

CREATE OR REPLACE FUNCTION enforce_chat_participant_rules_insert()
    RETURNS TRIGGER AS $$
DECLARE
    inviter             chat_inviter;
    chat_to_join        chat_where_user_joined;
    existing_owner_id   INTEGER;
    owner_count         INTEGER;
BEGIN
    IF NEW.role NOT IN ('owner', 'member', 'guest', 'bot') THEN
        RAISE EXCEPTION 'User % cannot join chat % with role %', NEW.user_id, NEW.chat_id, NEW.role;
    END IF;

    IF NEW.ban_reason IS NOT NULL OR
       NEW.ban_type IS NOT NULL OR
       NEW.banned_by IS NOT NULL OR
       NEW.banned_until IS NOT NULL OR
       NEW.ban_reason_note IS NOT NULL THEN
        RAISE EXCEPTION 'User % cannot join chat % in a banned state', NEW.user_id, NEW.chat_id;
    END IF;

    IF NEW.last_read_message_id IS NOT NULL OR NEW.last_read_at IS NOT NULL THEN
        RAISE EXCEPTION 'User % cannot join chat % while at the same time have read messages from it', NEW.user_id, NEW.chat_id;
    END IF;

    IF NEW.left_at IS NOT NULL OR NEW.rejoined_at IS NOT NULL THEN
        RAISE EXCEPTION 'User % cannot join chat % while at the same time leave (%) or rejoin (%) it',
            NEW.user_id, NEW.chat_id, NEW.left_at, NEW.rejoined_at;
    END IF;

    IF NEW.invited_by IS NOT NULL THEN
        SELECT role, permissions_bitmask
        INTO STRICT inviter
        FROM chat_participant
        WHERE chat_id = NEW.chat_id AND user_id = NEW.invited_by;

        IF get_bit(inviter.permissions_bitmask, 18) = 0 THEN
            RAISE EXCEPTION 'User % with role % cannot invite others to chat % without invite permission',
                NEW.invited_by, inviter.role, NEW.chat_id;
        END IF;
    END IF;

    -- Lock the chat row (to serialize concurrent inserts) and get the chat row
    SELECT type, visibility, status, created_at, created_by
    INTO STRICT chat_to_join
    FROM chat
    WHERE id = NEW.chat_id
    FOR UPDATE;

    IF chat_to_join.status = 'locked' THEN
        RAISE EXCEPTION 'User % cannot join chat % because its status is %',
            NEW.user_id, NEW.chat_id, chat_to_join.status;
    END IF;

    IF chat_to_join.visibility === 'secret' AND NEW.role <> 'owner' AND (NEW.invited_by IS NULL OR NEW.invited_at IS NULL) THEN
        RAISE EXCEPTION 'User % with role % cannot join secret chat % without invitation', NEW.user_id, NEW.role, NEW.chat_id;
    END IF;
    IF NEW.invited_at IS NOT NULL AND NEW.invited_at < chat_to_join.created_at THEN
        RAISE EXCEPTION 'User % cannot be invited to chat % at % before the chat was created at %',
            NEW.user_id, NEW.chat_id, NEW.invited_at, chat_to_join.created_at;
    END IF;

    IF NEW.joined_at < chat_to_join.created_at THEN
        RAISE EXCEPTION 'User % cannot join chat % at % before the chat was created at %',
            NEW.user_id, NEW.chat_id, NEW.joined_at, chat_to_join.created_at;
    END IF;

    IF NEW.role = 'owner' THEN
        IF chat_to_join.type = 'direct' THEN
            SELECT COUNT(*), MAX(user_id)
            INTO STRICT owner_count, existing_owner_id
            FROM chat_participant
            WHERE chat_id = NEW.chat_id AND role = 'owner';

            CASE owner_count
                WHEN 0 THEN
                    IF NEW.user_id <> chat_to_join.created_by THEN
                        RAISE EXCEPTION
                            'First owner of direct chat % must be the chat creator (user_id=%), but got user_id=%',
                            NEW.chat_id, chat_to_join.created_by, NEW.user_id;
                    END IF;
                WHEN 1 THEN
                    IF existing_owner_id = NEW.user_id THEN
                        RAISE EXCEPTION 'Direct chat % cannot have duplicate owners with user_id=%',
                            NEW.chat_id, NEW.user_id;
                    END IF;
                ELSE
                    RAISE EXCEPTION 'Direct chat % cannot have more than 2 owners', NEW.chat_id;
            END CASE;

        ELSIF NEW.user_id <> chat_to_join.created_by THEN
            RAISE EXCEPTION
                'Owner of chat "%" (%) must be the chat creator (user_id=%), but got user_id=%',
                chat_to_join.type, NEW.chat_id, chat_to_join.created_by, NEW.user_id;
            -- Index already enforces max 1 owner, so no need to count here
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER enforce_chat_participant_rules_insert_trigger
    BEFORE INSERT ON chat_participant
    FOR EACH ROW
EXECUTE FUNCTION enforce_chat_participant_rules_insert();

CREATE OR REPLACE FUNCTION enforce_chat_participant_immutability()
    RETURNS TRIGGER AS $$
DECLARE
    mutable_columns_after_user_left_chat TEXT[] = ARRAY['rejoined_at', 'banned_by', 'invited_by'];
BEGIN
    IF NEW.chat_type IS DISTINCT FROM OLD.chat_type THEN
        RAISE EXCEPTION 'chat_type of user % from chat % is immutable and cannot be changed from % to %',
            NEW.user_id, NEW.chat_id, OLD.chat_type, NEW.chat_type;
    END IF;

    IF NEW.joined_at IS DISTINCT FROM OLD.joined_at THEN
        RAISE EXCEPTION 'joined_at of user % from chat % is immutable and cannot be changed from % to %',
            NEW.user_id, NEW.chat_id, OLD.joined_at, NEW.joined_at;
    END IF;

    IF NEW.invited_at IS DISTINCT FROM OLD.invited_at THEN
        RAISE EXCEPTION 'invited_at of user % from chat % is immutable and cannot be changed from % to %',
            NEW.user_id, NEW.chat_id, OLD.invited_at, NEW.invited_at;
    END IF;

    IF NEW.invited_by IS DISTINCT FROM OLD.invited_by THEN
        IF OLD.invited_by IS NULL AND NEW.invited_by IS NOT NULL THEN
            RAISE EXCEPTION 'invited_by of user % from chat % is immutable and cannot be set from NULL to %',
                NEW.user_id, NEW.chat_id, NEW.invited_by;
        END IF;
        IF OLD.invited_by IS NOT NULL AND NEW.invited_by IS NULL THEN
            RAISE EXCEPTION 'invited_by of user % from chat % is immutable and cannot be changed from % to NULL',
                NEW.user_id, NEW.chat_id, OLD.invited_by;
        END IF;

        IF NOT EXISTS(
            SELECT 1
            FROM chat_participant
            WHERE chat_id = NEW.chat_id AND user_id = NEW.invited_by AND role = 'owner'
        ) THEN
            RAISE EXCEPTION 'invited_by % of user % from chat % must reference an owner in this chat',
                NEW.invited_by, NEW.user_id, NEW.chat_id;
        END IF;
    END IF;

    IF OLD.left_at IS NOT NULL THEN
        PERFORM check_whitelisted_updates(
                HSTORE(OLD), -- Convert the old row to hstore
                HSTORE(NEW), -- Convert the new row to hstore
                mutable_columns_after_user_left_chat
                );
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_participant_immutability_trigger
    BEFORE UPDATE OF chat_type, joined_at, invited_at, invited_by ON chat_participant
    FOR EACH ROW
EXECUTE FUNCTION enforce_chat_participant_immutability();

CREATE OR REPLACE FUNCTION enforce_ban_rules()
    RETURNS TRIGGER AS $$
DECLARE
    banner_permissions_bitmask BIGINT;
BEGIN
    IF NEW.chat_type IN ('direct', 'self') THEN
        IF NEW.ban_reason IS NOT NULL
            OR NEW.ban_type IS NOT NULL
            OR NEW.banned_by IS NOT NULL
            OR NEW.banned_until IS NOT NULL
            OR NEW.ban_reason_note IS NOT NULL
        THEN
            RAISE EXCEPTION 'User % from chat % of type % cannot be banned', NEW.user_id, NEW.chat_id, NEW.chat_type;
        END IF;

        RETURN NEW;
    END IF;

    IF NEW.ban_reason IS NOT NULL
        OR NEW.ban_type IS NOT NULL
        OR NEW.banned_until IS NOT NULL
        OR NEW.ban_reason_note IS NOT NULL
    THEN
        IF NEW.banned_by IS NULL THEN
            RAISE EXCEPTION 'banned_by is required when updating a ban of user % from chat %', NEW.user_id, NEW.chat_id;
        END IF;

        SELECT permissions_bitmask
        INTO STRICT banner_permissions_bitmask
        FROM chat_participant
        WHERE chat_id = NEW.chat_id AND user_id = NEW.banned_by;

        IF get_bit(banner_permissions_bitmask, 42) = 0 THEN
            RAISE EXCEPTION 'banned_by % of user % from chat % must have ban permission in this chat',
                NEW.banned_by, NEW.user_id, NEW.chat_id;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_participant_ban_check
    BEFORE UPDATE OF ban_reason, ban_type, banned_by, banned_until, ban_reason_note
    ON chat_participant
    FOR EACH ROW
EXECUTE FUNCTION enforce_ban_rules();

CREATE OR REPLACE FUNCTION enforce_role_transitions()
    RETURNS TRIGGER AS $$
BEGIN
    CASE
        WHEN OLD.role IN ('owner', 'bot') THEN
            RAISE EXCEPTION 'Role of user % from chat % is immutable: cannot change % to %',
                OLD.user_id, OLD.chat_id, OLD.role, NEW.role;
        WHEN NEW.role IN ('owner', 'guest', 'bot') THEN
            RAISE EXCEPTION 'Cannot change role of user % from chat % from % to %',
                OLD.user_id, OLD.chat_id, OLD.role, NEW.role;
        WHEN OLD.role = 'guest' AND NEW.role <> 'member' THEN
            RAISE EXCEPTION 'Role of user % from chat % can only change from guest to member, not to %',
                OLD.user_id, OLD.chat_id, NEW.role;
    END CASE;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_participant_role_transitions_trigger
    BEFORE UPDATE OF role ON chat_participant
    FOR EACH ROW
    WHEN (OLD.role IS DISTINCT FROM NEW.role)
EXECUTE FUNCTION enforce_role_transitions();

CREATE OR REPLACE FUNCTION enforce_participant_left_rejoined()
    RETURNS TRIGGER AS $$
DECLARE
    chat_status chat_status_enum;
BEGIN
    -- (rejoined_at, left_at) can only change in these transitions:
    -- (NULL, NULL) -> (NULL, NOT NULL)
    -- (NOT NULL, NULL) -> (NULL, NOT NULL)
    -- (NULL, NOT NULL) -> (NOT NULL, NULL)
    IF NOT (
        (((OLD.rejoined_at IS NULL AND OLD.left_at IS NULL) OR (OLD.rejoined_at IS NOT NULL AND OLD.left_at IS NULL))
            AND (NEW.rejoined_at IS NULL AND NEW.left_at IS NOT NULL)) OR
        ((OLD.rejoined_at IS NULL AND OLD.left_at IS NOT NULL) AND (NEW.rejoined_at IS NOT NULL AND NEW.left_at IS NULL))
        ) THEN
        RAISE EXCEPTION 'Invalid state transition of (rejoined_at, left_at) for participant % from chat %: from (%, %) to (%, %)',
            NEW.user_id, NEW.chat_id, OLD.rejoined_at, OLD.left_at, NEW.rejoined_at, NEW.left_at;
    END IF;

    IF NEW.left_at IS NOT NULL THEN
        IF OLD.chat_type IN ('direct', 'self') THEN
            RAISE EXCEPTION 'left_at (%) cannot be set for direct or self chats for participant % from chat %',
                NEW.left_at, OLD.user_id, OLD.chat_id;
        END IF;

        IF OLD.rejoined_at IS NOT NULL THEN
            IF NEW.left_at <= OLD.rejoined_at THEN
                RAISE EXCEPTION 'left_at (%) must be greater than rejoined_at (%) for participant % from chat %',
                    NEW.left_at, OLD.rejoined_at, OLD.user_id, OLD.chat_id;
            END IF;
        END IF;
    END IF;

    IF NEW.rejoined_at IS NOT NULL THEN
        IF OLD.chat_type IN ('direct', 'self') THEN
            RAISE EXCEPTION 'rejoined_at (%) cannot be set for direct or self chats for participant % from chat %',
                NEW.rejoined_at, OLD.user_id, OLD.chat_id;
        END IF;

        IF OLD.left_at IS NULL THEN
            RAISE EXCEPTION 'cannot set rejoined_at (%) without a prior left_at for participant % from chat %',
                NEW.rejoined_at, OLD.user_id, OLD.chat_id;
        END IF;

        IF NEW.rejoined_at <= OLD.left_at THEN
            RAISE EXCEPTION 'rejoined_at (%) must be greater than left_at (%) for participant % from chat %',
                NEW.rejoined_at, OLD.left_at, OLD.user_id, OLD.chat_id;
        END IF;

        SELECT status INTO STRICT chat_status FROM chat WHERE id = OLD.chat_id;
        IF chat_status = 'locked' THEN
            RAISE EXCEPTION 'user % cannot rejoin a locked chat % after leaving it', OLD.user_id, OLD.chat_id;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_participant_left_rejoined_check
    BEFORE UPDATE OF left_at, rejoined_at ON chat_participant
    FOR EACH ROW
    WHEN (NEW.left_at IS DISTINCT FROM OLD.left_at AND NEW.rejoined_at IS DISTINCT FROM OLD.rejoined_at)
EXECUTE FUNCTION enforce_participant_left_rejoined();

CREATE OR REPLACE FUNCTION chat_participant_sync_last_read()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.last_read_message_id IS NULL THEN
        NEW.last_read_at := NULL;
    ELSIF NEW.last_read_at IS NULL THEN
        NEW.last_read_at := NOW();
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER chat_participant_last_read_sync
    BEFORE UPDATE OF last_read_message_id ON chat_participant
    FOR EACH ROW
EXECUTE FUNCTION chat_participant_sync_last_read();

CREATE OR REPLACE FUNCTION validations_before_chat_participant_deletion()
    RETURNS TRIGGER AS $$
DECLARE
    owner_id INTEGER;
BEGIN
    -- Skip if triggered by cascade
    IF pg_trigger_depth() > 1 THEN
        RETURN OLD;
    END IF;

    IF OLD.role = 'owner' THEN
        RAISE EXCEPTION 'Cannot delete owner % from chat %, delete the chat instead.', OLD.user_id, OLD.chat_id;
    END IF;

    IF get_bit(OLD.permissions_bitmask, 18) = 1 OR get_bit(OLD.permissions_bitmask, 42) = 1 THEN
        SELECT user_id
        INTO STRICT owner_id
        FROM chat_participant
        WHERE chat_id = OLD.chat_id AND role = 'owner';

        UPDATE chat_participant
        SET invited_by = CASE WHEN invited_by = OLD.user_id THEN owner_id ELSE invited_by END,
            banned_by  = CASE WHEN banned_by  = OLD.user_id THEN owner_id ELSE banned_by  END
        WHERE chat_id = OLD.chat_id
          AND (invited_by = OLD.user_id OR banned_by = OLD.user_id);
    END IF;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER validations_before_chat_participant_deletion_trigger
    BEFORE DELETE ON chat_participant
    FOR EACH ROW
EXECUTE FUNCTION validations_before_chat_participant_deletion();