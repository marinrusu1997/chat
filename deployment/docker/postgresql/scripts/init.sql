-- This script will be executed ONCE on the master server's first startup
-- to create the initial database schema and extensions.
-- Patroni handles the creation of the replication user.
CREATE SCHEMA IF NOT EXISTS partman;

-- IMPORTANT: Extensions that affect schema and data (hstore, postgis, pg_partman)
-- must be created on the master. They will be replicated to the slaves.
CREATE EXTENSION IF NOT EXISTS hstore;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_partman WITH SCHEMA partman;

-- Set the search path so that PostgreSQL can find the functions for your extensions.
SET search_path = public, partman;

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
    password_updated_at     TIMESTAMPTZ CHECK (password_updated_at IS NULL OR (password_updated_at >= COALESCE(last_active_at, created_at) AND password_updated_at <= NOW() + INTERVAL '5 seconds')),
    -- PII
    name                    VARCHAR(255) NOT NULL CHECK (LENGTH(name) >= 2),
    -- activity
    last_login_at           TIMESTAMPTZ CHECK (last_login_at IS NULL OR (last_login_at > created_at AND last_login_at <= NOW() + INTERVAL '5 seconds')),
    last_active_at          TIMESTAMPTZ CHECK (last_active_at IS NULL OR (last_active_at >= COALESCE(last_login_at, created_at) AND last_active_at <= NOW() + INTERVAL '5 seconds')),
    created_at              TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL '5 seconds') DEFAULT NOW()
);

-- User Signal Keys, see https://en.wikipedia.org/wiki/Signal_Protocol

-- Role	        Description	                                Key Characteristics	                    Primary Use Case

-- primary	    The user's main device.	                    Cannot be automatically expired.    	A user's main phone.
--                                                          Can link other devices.
-- secondary	A trusted, long-term linked device.	        Can send/receive.                       A user's personal laptop or tablet.
--                                                          Expires after long inactivity.
-- read_only	A device that can only receive messages.    Cannot send messages.	                Audit terminals, public displays.
-- bot	        An automated agent.	                        Granular permissions.           	    Integrations, chatbots.
--                                                          Token-based auth.
-- ephemeral	A temporary, untrusted session.	            Very short expiry.                      Logging in on a public computer.
--                                                          Limited permissions.

CREATE TYPE chatting_device_role_enum AS ENUM ('primary', 'secondary', 'read_only', 'bot', 'ephemeral');
CREATE TABLE IF NOT EXISTS chatting_device
(
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    name                VARCHAR(50) NOT NULL CHECK (LENGTH(name) >= 2),
    role                chatting_device_role_enum NOT NULL,
    fingerprint         BYTEA NOT NULL CHECK (octet_length(fingerprint) = 32),
    created_at          TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL '5 seconds') DEFAULT NOW(),
    last_seen_at        TIMESTAMPTZ NOT NULL CHECK (
                            last_seen_at > created_at AND last_seen_at <= NOW() + INTERVAL '5 seconds'
                        ) DEFAULT NOW(),
    expires_at          TIMESTAMPTZ NOT NULL CHECK (expires_at > last_seen_at AND expires_at > NOW()),

    UNIQUE (user_id, name),
    UNIQUE (user_id, fingerprint),
    UNIQUE (name, fingerprint)
);

CREATE TABLE IF NOT EXISTS chatting_device_role_policy (
    role                    chatting_device_role_enum PRIMARY KEY,
    expiry_interval         INTERVAL NOT NULL CHECK (expiry_interval >= INTERVAL '1 minute')
);
INSERT INTO chatting_device_role_policy (role, expiry_interval)
VALUES
    ('secondary', '30 days'),
    ('read_only', '7 days'),
    ('bot',       '60 days'),
    ('ephemeral', '1 hour')
ON CONFLICT (role) DO UPDATE SET expiry_interval = EXCLUDED.expiry_interval;

CREATE TABLE IF NOT EXISTS device_signal_keys
(
    device_id                   BIGINT PRIMARY KEY REFERENCES chatting_device(id) ON DELETE CASCADE,
    user_id                     INTEGER NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    identity_key                BYTEA NOT NULL UNIQUE CHECK (octet_length(identity_key) = 32),
    signed_pre_key_id           SMALLINT NOT NULL CHECK (signed_pre_key_id > 0),
    signed_pre_key              BYTEA NOT NULL UNIQUE CHECK (octet_length(signed_pre_key) = 32),
    signed_pre_key_signature    BYTEA NOT NULL CHECK (octet_length(signed_pre_key_signature) = 64),
    created_at                  TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL '5 seconds') DEFAULT NOW(),
    last_refilled_at            TIMESTAMPTZ CHECK (last_refilled_at IS NULL OR (last_refilled_at > created_at AND last_refilled_at <= NOW() + INTERVAL '5 seconds'))
);

CREATE TABLE IF NOT EXISTS one_time_pre_key
(
    id          BIGINT PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,
    device_id   BIGINT NOT NULL REFERENCES device_signal_keys(device_id) ON DELETE CASCADE,
    public_key  BYTEA NOT NULL CHECK (octet_length(public_key) = 32),
    created_at  TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL '5 seconds') DEFAULT NOW(),

    UNIQUE (device_id, public_key)
);

CREATE TABLE IF NOT EXISTS one_time_pre_key_rate_limit
(
    user_id         INTEGER PRIMARY KEY REFERENCES "user"(id) ON DELETE CASCADE,
    tokens          DOUBLE PRECISION NOT NULL CHECK (tokens >= 0 AND tokens <= 50.0) DEFAULT 50.0,
    last_refill_ts  TIMESTAMPTZ NOT NULL CHECK (last_refill_ts <= NOW() + INTERVAL '5 seconds') DEFAULT NOW()
);

-- Session table
CREATE TABLE IF NOT EXISTS session
(
    id                  BIGSERIAL,
    user_id             INTEGER NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    refresh_token_hash  BYTEA NOT NULL CHECK (OCTET_LENGTH(refresh_token_hash) = 32),
    refresh_count       SMALLINT NOT NULL CHECK (refresh_count <= 5000) DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL '5 seconds') DEFAULT NOW(),
    expires_at          TIMESTAMPTZ NOT NULL CHECK (expires_at > created_at + INTERVAL '1 day'),
    ip                  INET NOT NULL,
    geo                 GEOGRAPHY(POINT) NOT NULL,
    user_agent          VARCHAR(512) CHECK (LENGTH(user_agent) > 0),
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

    PRIMARY KEY (id, expires_at)
) PARTITION BY RANGE (expires_at);

-- Chat table
CREATE TYPE chat_type_enum AS ENUM ('direct', 'group', 'self', 'thread');
CREATE TYPE chat_visibility_enum AS ENUM ('public', 'private', 'secret');
CREATE TYPE chat_post_policy_enum AS ENUM ('all', 'admins', 'owner');
CREATE TYPE chat_status_enum AS ENUM ('active', 'archived', 'locked');
CREATE TYPE chat_moderation_policy_enum AS ENUM ('none', 'flagged_review', 'auto_delete');
CREATE TYPE chat_encryption_enum AS ENUM ('at_rest', 'end_to_end');

CREATE TABLE IF NOT EXISTS chat
(
    id                  INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- enums
    type                chat_type_enum NOT NULL,
    visibility          chat_visibility_enum NOT NULL,
    post_policy         chat_post_policy_enum NOT NULL,
    status              chat_status_enum NOT NULL DEFAULT 'active',
    moderation_policy   chat_moderation_policy_enum NOT NULL DEFAULT 'none',
    encryption          chat_encryption_enum NOT NULL DEFAULT 'at_rest',
    -- settings / presentation
    name                VARCHAR(100) CHECK (name IS NULL OR LENGTH(name) >= 2),
    name_fts            tsvector GENERATED ALWAYS AS (
                            CASE
                                WHEN type = 'group' AND visibility = 'public' AND name IS NOT NULL
                                    THEN to_tsvector('english', name)
                            END
                        ) STORED,
    tags                VARCHAR(50)[] CHECK (cardinality(tags) >= 1 AND cardinality(tags) <= 10 AND NOT ('' = ANY(tags))),
    topic               VARCHAR(100) CHECK (topic IS NULL OR LENGTH(topic) >= 2),
    description         VARCHAR(500) CHECK (description IS NULL OR LENGTH(description) >= 2),
    settings            JSONB NOT NULL DEFAULT '{}',
    -- authorship / lineage
    created_by          INTEGER NOT NULL REFERENCES "user"(id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ NOT NULL CHECK (created_at <= NOW() + INTERVAL '5 seconds') DEFAULT NOW(),
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
    )),

    CHECK (encryption <> 'end_to_end' OR type IN ('direct', 'self')),

    CHECK (type = 'group' OR threads_enabled IS FALSE),
    CHECK ((type = 'thread') = (parent_id IS NOT NULL))
) PARTITION BY HASH (id);

-- Chat DEK history (versioned)
-- Stores the encrypted DEK for each chat, along with its validity period.
-- Get the current DEK with: SELECT * FROM chat_dek_history WHERE chat_id = $1 AND now() <@ valid_range;
CREATE TABLE chat_dek_history (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  chat_id        INTEGER NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
  encrypted_dek  BYTEA NOT NULL UNIQUE CHECK (octet_length(encrypted_dek) BETWEEN 32 AND 1024),
  dek_version    SMALLINT NOT NULL CHECK (dek_version >= 0),
  valid_from     TIMESTAMPTZ NOT NULL CHECK (valid_from <= NOW() + INTERVAL '5 days'),
  valid_to       TIMESTAMPTZ NOT NULL CHECK (valid_to >= valid_from + INTERVAL '31 days'),
  valid_range    tstzrange GENERATED ALWAYS AS (tstzrange(valid_from, valid_to)) STORED,

  UNIQUE(chat_id, dek_version),
  CONSTRAINT chat_dek_no_overlap EXCLUDE USING gist(chat_id WITH =, valid_range WITH &&)
);

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
    joined_at               TIMESTAMPTZ NOT NULL CHECK (joined_at <= NOW() + INTERVAL '5 seconds') DEFAULT NOW(),
    rejoined_at             TIMESTAMPTZ CHECK (rejoined_at IS NULL OR (rejoined_at > joined_at AND rejoined_at <= NOW() + INTERVAL '5 seconds')),
    left_at                 TIMESTAMPTZ CHECK (left_at IS NULL OR (left_at > joined_at AND left_at <= NOW() + INTERVAL '5 seconds')),
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
    last_read_at            TIMESTAMPTZ CHECK (last_read_at IS NULL OR (last_read_at > joined_at AND last_read_at <= NOW() + INTERVAL '5 seconds')),
    -- notifications & preferences
    muted_until             TIMESTAMPTZ CHECK (muted_until IS NULL OR (muted_until > NOW() + INTERVAL '5 seconds')),
    notification_level      chat_participant_notification_level_enum NOT NULL DEFAULT 'all',
    custom_nickname         VARCHAR(100) CHECK (LENGTH(custom_nickname) >= 1),
    color_theme             VARCHAR(50) CHECK (LENGTH(color_theme) >= 1),
    settings                JSONB NOT NULL DEFAULT '{}',
    -- pinning & tagging
    is_pinned               BOOLEAN NOT NULL DEFAULT FALSE,
    last_pinned_message_id  UUID,

    PRIMARY KEY (user_id, chat_id), -- to find all chats of user

    CHECK (
        (role = 'guest' AND permissions_bitmask = B'0000000000000000000000000000000000000000000000000000000000000000') OR
        (role = 'bot' AND (permissions_bitmask & B'0000000000000000000000000000000000000000000000000000000111111111') = permissions_bitmask) OR
        (role = 'member' AND (permissions_bitmask & B'0000000000000000000000000000000000000000000000000000000111111111') = permissions_bitmask) OR
        (role = 'moderator' AND (permissions_bitmask & B'0000000000000000000000000000000000000000000000000011111111111111') = permissions_bitmask) OR
        (role = 'admin' AND (permissions_bitmask & B'0000000000000000000000000000000000000000000000001111111111111111') = permissions_bitmask) OR
        (role = 'owner' AND permissions_bitmask = B'1111111111111111111111111111111111111111111111111111111111111111')
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

-- Indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_email                    ON "user"(email) INCLUDE (password_hash, password_algo);

CREATE INDEX IF NOT EXISTS idx_devices_user_id                      ON chatting_device(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_primary_chat_device_per_user   ON chatting_device(user_id) WHERE (role = 'primary');
CREATE INDEX IF NOT EXISTS idx_devices_expires_at                   ON chatting_device(expires_at);

CREATE INDEX IF NOT EXISTS idx_one_time_pre_keys_device_id          ON one_time_pre_key(device_id);

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

-- User Signal Keys
CREATE OR REPLACE FUNCTION tgf_prevent_update_on_expired_device()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
BEGIN
    -- Check the state of the row as it currently exists in the table.
    -- We use OLD.expires_at because we want to prevent updates on rows that are *already* expired.
    IF OLD.expires_at <= NOW() THEN
        RAISE EXCEPTION 'Cannot update device % because its session has expired at %.',
            OLD.id, OLD.expires_at;
    END IF;

    -- If the device is not expired, allow the UPDATE to proceed.
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_prevent_update_on_expired_device
    BEFORE UPDATE ON chatting_device
    FOR EACH ROW
EXECUTE FUNCTION tgf_prevent_update_on_expired_device();

CREATE OR REPLACE FUNCTION tgf_chatting_device_ensure_immutability()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
BEGIN
    -- Enforce immutability for critical fields.
    IF OLD.user_id IS DISTINCT FROM NEW.user_id THEN
        RAISE EXCEPTION 'cannot change the user_id of a chatting device from "%" to "%"', OLD.user_id, NEW.user_id;
    END IF;
    IF OLD.name IS DISTINCT FROM NEW.name THEN
        RAISE EXCEPTION 'cannot change the name of a chatting device from "%" to "%"', OLD.name, NEW.name;
    END IF;
    IF OLD.role IS DISTINCT FROM NEW.role THEN
        RAISE EXCEPTION 'cannot change the role of a chatting device from "%" to "%"', OLD.role, NEW.role;
    END IF;
    IF OLD.fingerprint IS DISTINCT FROM NEW.fingerprint THEN
        RAISE EXCEPTION 'cannot change the fingerprint of a chatting device from "%" to "%"', OLD.fingerprint, NEW.fingerprint;
    END IF;
    IF OLD.created_at IS DISTINCT FROM NEW.created_at THEN
        RAISE EXCEPTION 'cannot change the created_at timestamp of a chatting device from "%" to "%"', OLD.created_at, NEW.created_at;
    END IF;

    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_chatting_device_ensure_immutability
    BEFORE UPDATE OF user_id, name, role, fingerprint, created_at ON chatting_device
    FOR EACH ROW
EXECUTE FUNCTION tgf_chatting_device_ensure_immutability();

CREATE OR REPLACE FUNCTION tgf_chatting_device_monotonic_last_seen()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.last_seen_at <= OLD.last_seen_at THEN
        RAISE EXCEPTION 'The new last_seen_at timestamp (%) from device % must be greater than the previous one (%).',
            NEW.last_seen_at, NEW.id, OLD.last_seen_at;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_chatting_device_monotonic_last_seen
    BEFORE UPDATE OF last_seen_at ON chatting_device
    FOR EACH ROW
EXECUTE FUNCTION tgf_chatting_device_monotonic_last_seen();

CREATE OR REPLACE FUNCTION tgf_chatting_device_max_count_statement()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
DECLARE
    v_max_devices_per_user      SMALLINT := 10;
    v_chatting_device_record    RECORD;
    v_current_device_count      SMALLINT;
    v_new_device_count          BIGINT;
BEGIN
    FOR v_chatting_device_record IN SELECT DISTINCT user_id FROM inserted_rows
        LOOP
            -- CRITICAL: CONCURRENCY LOCK
            -- Even in a statement-level trigger, we must lock per-user to prevent a race
            -- condition where two concurrent batch inserts for the same user are processed.
            -- The advisory lock serializes the checks for each specific user.
            PERFORM pg_advisory_xact_lock(v_chatting_device_record.user_id);

            -- Now that we have a lock, we can safely perform our checks for this user.

            -- 1. Count how many new devices are being added for THIS user in THIS batch.
            SELECT COUNT(*) INTO v_new_device_count
            FROM inserted_rows
            WHERE user_id = v_chatting_device_record.user_id;

            -- 2. Count how many devices THIS user already has in the main table.
            SELECT COUNT(*) INTO v_current_device_count
            FROM chatting_device
            WHERE user_id = v_chatting_device_record.user_id AND expires_at > NOW();

            -- 3. Check if the total would exceed the limit.
            IF (v_current_device_count + v_new_device_count) > v_max_devices_per_user THEN
                RAISE EXCEPTION 'User % cannot register % new device(s): it would exceed the maximum limit of %. (Current device count: %)',
                    v_chatting_device_record.user_id, v_new_device_count, v_max_devices_per_user, v_current_device_count;
            END IF;

        END LOOP;

    -- For a BEFORE statement trigger, the return value is ignored, but we return NULL by convention.
    RETURN NULL;
END;
$$;
CREATE TRIGGER trg_chatting_device_max_count
    AFTER INSERT ON chatting_device
    REFERENCING NEW TABLE AS inserted_rows
    FOR EACH STATEMENT -- This clause ensures the trigger fires only ONCE for the entire INSERT statement.
EXECUTE FUNCTION tgf_chatting_device_max_count_statement();

CREATE OR REPLACE FUNCTION tgf_set_device_expiry()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
DECLARE
    v_expiry_interval INTERVAL;
BEGIN
    -- For 'primary' devices, expiry is not applicable. Set to NULL and exit.
    IF NEW.role = 'primary' THEN
        NEW.expires_at := 'infinity';
        RETURN NEW;
    END IF;

    -- For all other roles, we only need to re-calculate the expiry if it's a new device,
    -- or if the 'last_seen_at' timestamp has actually been updated.
    -- This is a performance optimization to prevent the trigger from doing work
    -- on unrelated UPDATEs (e.g., changing the device name).
    IF TG_OP = 'INSERT' OR NEW.last_seen_at IS DISTINCT FROM OLD.last_seen_at THEN
        BEGIN
            -- Attempt to fetch the policy for this device's role.
            -- INTO STRICT will raise NO_DATA_FOUND if no policy exists.
            SELECT expiry_interval
            INTO STRICT v_expiry_interval
            FROM chatting_device_role_policy
            WHERE role = NEW.role;

            -- If the above query succeeded, calculate the new expiry date.
            NEW.expires_at := NEW.last_seen_at + v_expiry_interval;
        EXCEPTION
            -- If the STRICT query failed, it means we have a device role
            -- that has no defined expiry policy. This is a configuration error.
            WHEN NO_DATA_FOUND THEN
                RAISE EXCEPTION 'No expiry policy found in device_role_policies for role: %. Please add a policy for this role.', NEW.role;
        END;
    END IF;

    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_set_device_expiry
    BEFORE INSERT OR UPDATE OF last_seen_at ON chatting_device
    FOR EACH ROW
EXECUTE FUNCTION tgf_set_device_expiry();

CREATE OR REPLACE FUNCTION prune_stale_chatting_devices()
    RETURNS void
    LANGUAGE plpgsql
    SECURITY DEFINER
AS $$
BEGIN
    DELETE FROM chatting_device WHERE expires_at <= NOW();
END;
$$;

CREATE OR REPLACE FUNCTION check_device_is_active(
    p_device_id INTEGER
)
    RETURNS void
    LANGUAGE plpgsql
    STABLE -- It's STABLE because it depends on NOW(), but not VOLATILE.
AS $$
DECLARE
    v_is_valid BOOLEAN;
BEGIN
    BEGIN
        SELECT TRUE
        INTO STRICT v_is_valid
        FROM chatting_device
        WHERE id = p_device_id AND expires_at > NOW();
    EXCEPTION
        WHEN NO_DATA_FOUND THEN
            RAISE EXCEPTION 'Device % is either not found or has expired.', p_device_id;
    END;
END;
$$;
COMMENT ON FUNCTION check_device_is_active(INTEGER) IS $$
Checks if a given device ID exists and has not expired.
Raises an EXCEPTION if the device is not found or is expired.
Returns void on success.
$$;

CREATE OR REPLACE FUNCTION tgf_validate_device_signal_keys_write()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
BEGIN
    -- CHECK 1: This check runs for BOTH INSERT and UPDATE.
    PERFORM check_device_is_active(NEW.device_id);

    -- CHECK 2: This check should ONLY run for an INSERT.
    IF TG_OP = 'INSERT' THEN
        IF NEW.last_refilled_at IS NOT NULL THEN
            RAISE EXCEPTION '"last_refilled_at" must be NULL on initial insertion of device keys. Value provided: %',
                NEW.last_refilled_at;
        END IF;
    END IF;

    -- If all checks pass, allow the operation to proceed.
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_validate_device_signal_keys_before_write
    BEFORE INSERT OR UPDATE ON device_signal_keys
    FOR EACH ROW
EXECUTE FUNCTION tgf_validate_device_signal_keys_write();

CREATE OR REPLACE FUNCTION tgf_device_signal_keys_immutability()
    RETURNS TRIGGER AS $$
BEGIN
    IF OLD.user_id IS DISTINCT FROM NEW.user_id THEN
        RAISE EXCEPTION 'user_id is immutable and cannot be changed for device % and user %.',
            OLD.device_id, OLD.user_id;
    END IF;

    IF OLD.identity_key IS DISTINCT FROM NEW.identity_key THEN
        RAISE EXCEPTION 'identity_key is immutable and cannot be changed for device % and user %.',
            OLD.device_id, OLD.user_id;
    END IF;

    IF OLD.created_at IS DISTINCT FROM NEW.created_at THEN
        RAISE EXCEPTION 'created_at is immutable and cannot be changed for device % and user %.',
            OLD.device_id, OLD.user_id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER device_signal_keys_immutability_trigger
    BEFORE UPDATE OF user_id, identity_key, created_at ON device_signal_keys
    FOR EACH ROW
EXECUTE FUNCTION tgf_device_signal_keys_immutability();

CREATE OR REPLACE FUNCTION tgf_device_signal_keys_signed_pre_key_checks()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.signed_pre_key_id IS NULL OR NEW.signed_pre_key IS NULL OR NEW.signed_pre_key_signature IS NULL THEN
        RAISE EXCEPTION 'signed_pre_key_id=%, signed_pre_key=% and signed_pre_key_signature=% need to be updated all at once.',
            NEW.signed_pre_key_id, NEW.signed_pre_key, NEW.signed_pre_key_signature;
    END IF;

    IF NEW.signed_pre_key_id <= OLD.signed_pre_key_id THEN
        RAISE EXCEPTION 'new signed_pre_key_id=% must be greater than the previous=%.',
            NEW.signed_pre_key_id, OLD.signed_pre_key_id;
    END IF;

    IF NEW.signed_pre_key = OLD.signed_pre_key THEN
        RAISE EXCEPTION 'new signed_pre_key must be different from the old one.';
    END IF;

    IF NEW.signed_pre_key_signature = OLD.signed_pre_key_signature THEN
        RAISE EXCEPTION 'new signed_pre_key_signature must be different from the old one.';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER user_device_keys_signed_pre_key_checks_trigger
    BEFORE UPDATE OF signed_pre_key_id, signed_pre_key, signed_pre_key_signature ON device_signal_keys
    FOR EACH ROW
EXECUTE FUNCTION tgf_device_signal_keys_signed_pre_key_checks();

CREATE OR REPLACE FUNCTION tgf_device_signal_keys_refill_checks()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.last_refilled_at <= OLD.last_refilled_at THEN
        RAISE EXCEPTION 'new last_refilled_at % must be monotonically increasing from %.', NEW.last_refilled_at, OLD.last_refilled_at;
    END IF;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER user_device_keys_refill_checks_trigger
    BEFORE UPDATE OF last_refilled_at ON device_signal_keys
    FOR EACH ROW
    WHEN (NEW.last_refilled_at IS NOT NULL AND OLD.last_refilled_at IS NOT NULL)
EXECUTE FUNCTION tgf_device_signal_keys_refill_checks();

CREATE OR REPLACE FUNCTION tgf_one_time_pre_keys_immutable()
    RETURNS TRIGGER AS $$
BEGIN
    -- An existing one-time key should never be modified. It should only be inserted and deleted.
    RAISE EXCEPTION 'One-time pre-keys are immutable and cannot be updated.';
    RETURN NEW; -- This line is technically unreachable but required by the function signature
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_one_time_pre_keys_prevent_update
    BEFORE UPDATE ON one_time_pre_key
    FOR EACH ROW
EXECUTE FUNCTION tgf_one_time_pre_keys_immutable();

CREATE OR REPLACE FUNCTION tgf_update_last_refilled_at()
    RETURNS TRIGGER AS $$
BEGIN
    UPDATE device_signal_keys
    SET last_refilled_at = NOW()
    WHERE device_id IN (
        SELECT DISTINCT device_id FROM inserted_rows
    );

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_one_time_pre_keys_after_insert
    AFTER INSERT ON one_time_pre_key
    REFERENCING NEW TABLE AS inserted_rows
    FOR EACH STATEMENT
EXECUTE FUNCTION tgf_update_last_refilled_at();

CREATE OR REPLACE FUNCTION tgf_check_one_time_pre_key_limit()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS $$
DECLARE
    v_max_keys_per_device   SMALLINT := 500;
    v_one_time_key_record   RECORD;
    v_current_key_count     SMALLINT;
    v_new_key_count         BIGINT;
BEGIN
    FOR v_one_time_key_record IN SELECT DISTINCT device_id FROM inserted_rows LOOP
        PERFORM check_device_is_active(v_one_time_key_record.device_id);

        -- CRITICAL: CONCURRENCY LOCK
        -- To prevent a race condition where two transactions check the count for the same
        -- user at the same time, we MUST acquire an exclusive lock for this user.
        -- The safest place to lock is the parent row in `device_signal_keys`.
        -- This forces any other transaction trying to add keys for the SAME user to wait.
        PERFORM * FROM device_signal_keys WHERE device_id = v_one_time_key_record.device_id FOR UPDATE;

        -- Now that we have a lock, we can safely perform our checks for this user.

        -- 1. Count how many keys are being added for this user in this batch.
        SELECT COUNT(*) INTO v_new_key_count
        FROM inserted_rows
        WHERE device_id = v_one_time_key_record.device_id;

        -- 2. Count how many keys already exist in the table for this user.
        SELECT COUNT(*) INTO v_current_key_count
        FROM one_time_pre_key
        WHERE device_id = v_one_time_key_record.device_id;

        -- 3. Check if the total would exceed the limit.
        IF (v_current_key_count + v_new_key_count) > v_max_keys_per_device THEN
            RAISE EXCEPTION 'Cannot add % new signal one time public keys for device %:
                it would exceed the maximum limit of % keys. (Current count: %)',
                v_new_key_count, v_one_time_key_record.device_id, v_max_keys_per_device, v_current_key_count;
        END IF;
    END LOOP;

    -- For a BEFORE statement trigger, the return value is ignored, but we return NULL by convention.
    RETURN NULL;
END;
$$;
CREATE TRIGGER trg_check_one_time_pre_key_limit
    AFTER INSERT ON one_time_pre_key
    REFERENCING NEW TABLE AS inserted_rows
    FOR EACH STATEMENT -- ensures the trigger fires only ONCE for a batch insert.
EXECUTE FUNCTION tgf_check_one_time_pre_key_limit();

CREATE TYPE device_pre_key_bundle AS (
    device_id                   BIGINT,
    identity_key                BYTEA,
    signed_pre_key_id           SMALLINT,
    signed_pre_key              BYTEA,
    signed_pre_key_signature    BYTEA,
    one_time_pre_key            BYTEA
);
CREATE TYPE device_base_bundle AS (
    device_id                   BIGINT,
    identity_key                BYTEA,
    signed_pre_key_id           SMALLINT,
    signed_pre_key              BYTEA,
    signed_pre_key_signature    BYTEA
);
CREATE OR REPLACE FUNCTION get_user_signal_pre_key_bundle(
    p_sender_id     INTEGER,    -- The ID of the user REQUESTING the bundle
    p_receiver_id   INTEGER,    -- The ID of the user WHOSE bundle is being requested
    p_chat_id       INTEGER     -- The ID of the chat where the sender wants to send a message
)
    RETURNS SETOF device_pre_key_bundle
    LANGUAGE plpgsql
    VOLATILE -- VOLATILE is the default, but it's good to be explicit. It means the function has side effects.
AS $$
DECLARE
    -- Configuration for the token bucket algorithm
    v_bucket_capacity         DOUBLE PRECISION := 50.0; -- Max burst of 50 requests.
    v_refill_rate_per_sec     DOUBLE PRECISION := (200.0 / 3600.0); -- 200 requests per hour.

    -- Variables to hold state
    v_rate_limit_state        one_time_pre_key_rate_limit%ROWTYPE;
    v_bundle_base             device_base_bundle;
    v_one_time_public_key     BYTEA;
    v_final_bundle            device_pre_key_bundle;
    v_current_ts              TIMESTAMPTZ := NOW();
    v_time_elapsed            DOUBLE PRECISION;
    v_new_tokens              DOUBLE PRECISION;
    v_current_tokens          DOUBLE PRECISION;
    v_chat_type               chat_type_enum;
    v_device_found            BOOLEAN := FALSE;
BEGIN
    -- ========= 1. Context and Sanity Checks ==========
    -- Check 1: Validate chat
    SELECT type
    INTO STRICT v_chat_type
    FROM chat
    WHERE id = p_chat_id AND encryption = 'end_to_end';

    --- Check 2: Validate participants
    CASE v_chat_type
        WHEN 'self' THEN
            IF p_sender_id <> p_receiver_id THEN
                RAISE EXCEPTION 'In a self chat %, the sender (%) and receiver (%) must be the same user.',
                    p_chat_id, p_sender_id, p_receiver_id;
            END IF;

            PERFORM 1 FROM chat_participant WHERE chat_id = p_chat_id AND user_id = p_receiver_id;
        WHEN 'direct' THEN
            IF p_sender_id = p_receiver_id THEN
                PERFORM 1 FROM chat_participant WHERE chat_id = p_chat_id AND user_id = p_receiver_id; -- user sends to his own devices
            ELSE
                PERFORM 1 FROM chat_participant
                    WHERE chat_id = p_chat_id AND user_id IN (p_sender_id, p_receiver_id)
                    GROUP BY chat_id
                    HAVING COUNT(user_id) = 2;
            END IF;
        ELSE
            RAISE EXCEPTION 'Chat % is of type %, which is not supported for end-to-end encryption.',
                p_chat_id, v_chat_type;
    END CASE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Sender (%) and/or receiver (%) are not valid participants in chat %.', p_sender_id, p_receiver_id, p_chat_id;
    END IF;

    -- ========= 2. Rate Limiting Logic for the Sender =========
    -- Lock the sender's rate limit state row to prevent race conditions from concurrent requests by the SAME sender.
    INSERT INTO one_time_pre_key_rate_limit(user_id, tokens, last_refill_ts)
    VALUES (p_sender_id, v_bucket_capacity, v_current_ts)
    ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id -- No-op to enable RETURNING on existing rows
    RETURNING * INTO STRICT v_rate_limit_state;

    v_time_elapsed := EXTRACT(EPOCH FROM (v_current_ts - v_rate_limit_state.last_refill_ts));
    v_new_tokens := v_time_elapsed * v_refill_rate_per_sec;
    v_current_tokens := LEAST(v_bucket_capacity, v_rate_limit_state.tokens + v_new_tokens);

    IF v_current_tokens < 1.0 THEN
        RAISE EXCEPTION 'Rate limit exceeded for user %. Try again later.
                Sender % tried to obtain Signal Pre Key Bundle of receiver % while attempting to send a message in chat %',
            p_sender_id, p_sender_id, p_receiver_id, p_chat_id;
    END IF;

    UPDATE one_time_pre_key_rate_limit
    SET tokens = v_current_tokens - 1.0, last_refill_ts = v_current_ts
    WHERE user_id = p_sender_id;

    -- ========= 3. Atomically Fetch Bundles for EACH of the Receiver's Devices =========
    FOR v_bundle_base IN
        SELECT
            dsk.device_id,
            dsk.identity_key,
            dsk.signed_pre_key_id,
            dsk.signed_pre_key,
            dsk.signed_pre_key_signature
        FROM device_signal_keys dsk JOIN chatting_device d ON dsk.device_id = d.id
        WHERE dsk.user_id = p_receiver_id AND d.expires_at > NOW()
    LOOP
        v_device_found := TRUE;

        DELETE FROM one_time_pre_key
        WHERE id = (
            SELECT id
            FROM one_time_pre_key
            WHERE device_id = v_bundle_base.device_id
            ORDER BY id
            LIMIT 1
            FOR UPDATE SKIP LOCKED
        )
        RETURNING public_key INTO STRICT v_one_time_public_key;

        v_final_bundle.device_id                 := v_bundle_base.device_id;
        v_final_bundle.identity_key              := v_bundle_base.identity_key;
        v_final_bundle.signed_pre_key_id         := v_bundle_base.signed_pre_key_id;
        v_final_bundle.signed_pre_key            := v_bundle_base.signed_pre_key;
        v_final_bundle.signed_pre_key_signature  := v_bundle_base.signed_pre_key_signature;
        v_final_bundle.one_time_pre_key          := v_one_time_public_key;

        RETURN NEXT v_final_bundle;
    END LOOP;

    IF NOT v_device_found THEN
        RAISE EXCEPTION 'Receiver user % has no devices with registered signal keys
            when sender % requested signal pre_key bundle to initiate session in chat %.',
            p_receiver_id, p_sender_id, p_chat_id;
    END IF;

    RETURN;
END;
$$;
COMMENT ON FUNCTION get_user_signal_pre_key_bundle(INTEGER, INTEGER, INTEGER) IS $$
Atomically fetches all necessary pre_key bundles for a receiver's devices to initiate a secure session.

This function serves as the secure gateway for establishing an end-to-end encrypted session for 'direct' or 'self' chats.
It follows a strict "validate first, act second" principle.

The execution flow is as follows:
1.  **Validation:** It first performs a series of rigorous checks to ensure the request is valid.
    This includes verifying the chat's existence, its encryption status ('end_to_end'), its type ('direct' or 'self'),
    and that the sender/receiver are legitimate participants.
2.  **Rate Limiting:** If validation passes, it then applies a per-user rate limit using a token bucket algorithm to prevent abuse.
3.  **Bundle Retrieval:** Finally, it iterates through all of the receiver's registered devices and
    atomically fetches and consumes one one-time pre_key for each, assembling a complete bundle.

This function ENFORCES the use of a one-time pre_key and does not support the protocol's standard fallback mechanism.

@args
    p_sender_id (INTEGER): The ID of the user REQUESTING the bundles.
    p_chat_id (INTEGER): The ID of the 'direct' or 'self' chat that provides the context for the request.
    p_receiver_id (INTEGER): The ID of the user WHOSE device bundles are being requested.

@returns
    SETOF device_pre_key_bundle: A table-like result set where each row is a complete pre_key bundle for one of the receiver's devices.
                                 The `device_id` in each row identifies the specific device the bundle belongs to.

@raises
    EXCEPTION on any of the following validation failures:
        - If the chat does not exist or is not 'end_to_end' encrypted.
        - If the chat type is not 'direct' or 'self'.
        - If the participants are not valid for the given chat type.
        - If the rate limit is exceeded for the sender.
        - If the receiver user has no devices with registered signal keys.
        - If any of the receiver's devices have run out of available one-time pre_keys.
$$;

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

CREATE OR REPLACE FUNCTION tgf_prevent_modification_of_session()
    RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Cannot modify session % of user %', OLD.id, OLD.user_id;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER tgr_enforce_readonly_session_row
    BEFORE UPDATE ON session
    FOR EACH ROW
EXECUTE FUNCTION tgf_prevent_modification_of_session();

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
-- 2) Immutable columns: type, encryption, name, created_by, created_at, parent_id, threads_enabled
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
    IF NEW.encryption IS DISTINCT FROM OLD.encryption THEN
        changed_cols := changed_cols || ' encryption';
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
    BEFORE UPDATE OF type, encryption, name, created_by, created_at, parent_id, threads_enabled ON chat
    FOR EACH ROW
EXECUTE FUNCTION chat_immutables_guard();

-- ============================================================
-- 3) Type-specific constraints on UPDATE
--    Parent-related checks done here for threads.
-- ============================================================
CREATE OR REPLACE FUNCTION chat_parent_constraints()
    RETURNS TRIGGER AS $$
DECLARE
    parent_row chat%ROWTYPE;
BEGIN
    SELECT *
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

CREATE OR REPLACE FUNCTION chat_can_have_threads(p_chat chat)
    RETURNS BOOLEAN AS $$
        SELECT p_chat.type = 'group' AND
               p_chat.threads_enabled = TRUE;
$$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION chat_type_rules_insert()
    RETURNS TRIGGER AS $$
DECLARE
    parent_row chat%ROWTYPE;
BEGIN
    -- Default status
    NEW.status := 'active';

    IF NEW.type == 'thread' THEN
        SELECT *
        INTO STRICT parent_row
        FROM chat
        WHERE id = NEW.parent_id;

        IF chat_can_have_threads(parent_row) IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'thread (%): parent (%) must be an existing group with threads_enabled=TRUE',
                NEW.id, NEW.parent_id;
        END IF;
        IF parent_row.status <> 'active' THEN
            RAISE EXCEPTION 'thread (%): parent (%) must be active when adding a thread, not "%" ',
                NEW.id, NEW.parent_id, parent_row.status;
        END IF;

        -- Visibility bounds
        IF parent_row.visibility = 'private' AND NOT (NEW.visibility IN ('private', 'secret')) THEN
            RAISE EXCEPTION 'thread (%): visibility "%" cannot exceed parent (%) visibility "%"',
                NEW.id, NEW.visibility, NEW.parent_id, parent_row.visibility;
        ELSIF parent_row.visibility = 'secret' AND NEW.visibility != 'secret' THEN
            RAISE EXCEPTION 'thread (%): visibility "%" cannot exceed parent (%) visibility "%"',
                NEW.id, NEW.visibility, NEW.parent_id, parent_row.visibility;
        END IF;

        -- Post policy bounds
        IF parent_row.post_policy = 'all' AND NEW.post_policy != 'all' THEN
            RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
                NEW.id, NEW.post_policy, NEW.parent_id, parent_row.post_policy;
        ELSIF parent_row.post_policy = 'admins' AND NOT (NEW.post_policy IN ('all','admins')) THEN
            RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
                NEW.id, NEW.post_policy, NEW.parent_id, parent_row.post_policy;
        ELSIF parent_row.post_policy = 'owner' AND NOT (NEW.post_policy IN ('all','owner')) THEN
            RAISE EXCEPTION 'thread (%): post_policy "%" cannot exceed parent (%) post_policy "%"',
                NEW.id, NEW.post_policy, NEW.parent_id, parent_row.post_policy;
        END IF;

        -- Moderation policy bounds
        IF parent_row.moderation_policy IN ('flagged_review','auto_delete') AND NEW.moderation_policy != 'auto_delete' THEN
            RAISE EXCEPTION 'thread (%): moderation_policy "%" cannot exceed parent (%) moderation_policy "%"',
                NEW.id, NEW.moderation_policy, NEW.parent_id, parent_row.moderation_policy;
        END IF;

        -- Expires At bound
        IF parent_row.expires_at IS NOT NULL AND (NEW.expires_at IS NULL OR NEW.expires_at > parent_row.expires_at) THEN
            NEW.expires_at = parent_row.expires_at;
        END IF;

        -- Inherit parent properties
        NEW.created_by = parent_row.created_by;
        NEW.encryption = parent_row.encryption;
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

-- Chat DEK
CREATE OR REPLACE FUNCTION tgf_prevent_modification_of_chat_dek_history()
    RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Cannot modify chat dek history row % of chat %', OLD.id, OLD.chat_id;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER tgr_enforce_readonly_chat_dek_history_row
    BEFORE UPDATE ON chat_dek_history
    FOR EACH ROW
EXECUTE FUNCTION tgf_prevent_modification_of_chat_dek_history();

CREATE OR REPLACE FUNCTION tgf_enforce_chat_dek_insert_rules()
    RETURNS TRIGGER AS $$
DECLARE
    max_version SMALLINT;
BEGIN
    -- Check valid interval
    IF NOT EXISTS ( -- if there is no prev interval where we can anchor to
        SELECT 1 FROM chat_dek_history
        WHERE chat_id = NEW.chat_id AND valid_to = NEW.valid_from
    ) THEN
        IF EXISTS ( -- if it’s NOT the very first interval for the chat
            SELECT 1 FROM chat_dek_history WHERE chat_id = NEW.chat_id
        ) THEN
            RAISE EXCEPTION 'Intervals must be contiguous (no gaps) for chat %. Trying to add interval from % to %',
                NEW.chat_id, NEW.valid_from, NEW.valid_to;
        END IF;
    END IF;

    -- Check versioning
    SELECT COALESCE(MAX(dek_version), -1)
    INTO max_version
    FROM chat_dek_history
    WHERE chat_id = NEW.chat_id;

    IF NEW.dek_version IS NULL THEN
        NEW.dek_version := max_version + 1;
    ELSE
        IF NEW.dek_version <= max_version THEN
            RAISE EXCEPTION 'dek_version % must be greater than current max % for chat_id %',
                NEW.dek_version, max_version, NEW.chat_id;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_enforce_chat_dek_insert_rules
    BEFORE INSERT ON chat_dek_history
    FOR EACH ROW
EXECUTE FUNCTION tgf_enforce_chat_dek_insert_rules();

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

-- # Partitions
-- session
CREATE TABLE session_pman_tmpl (LIKE session);
ALTER TABLE session_pman_tmpl SET (
    autovacuum_vacuum_scale_factor = 0.0,
    autovacuum_vacuum_threshold = 100000,
    autovacuum_analyze_scale_factor = 0.05,
    autovacuum_analyze_threshold = 50
);

SELECT partman.create_parent(
   p_parent_table           := 'public.session',
   p_template_table         := 'public.session_pman_tmpl',
   p_control                := 'expires_at',
   p_interval               := '1 day',
   p_type                   := 'range',
   p_premake                := 4,
   p_default_table          := false,
   p_automatic_maintenance  := 'on',
   p_constraint_cols        := ARRAY['id', 'user_id', 'refresh_token_hash'],
   p_jobmon                 := false
);

UPDATE partman.part_config
SET
    retention = '21 days', -- keep 3 weeks of sessions, assuming a session is valid for 2 weeks
    retention_keep_table = false,
    retention_keep_index = false,
    retention_keep_publication = false,
    optimize_constraint = 1, -- add constraints for sessions older than 1 day
    constraint_valid = true,
    ignore_default_data = true
WHERE parent_table = 'public.session';

-- chat
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

-- chat_participant
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