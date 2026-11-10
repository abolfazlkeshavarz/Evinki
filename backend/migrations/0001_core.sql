-- 0001 core: identity, access control, audit, files, messaging, settings.
-- Conventions for the whole schema:
--   * ids are BIGINT identities, money is whole Toman in BIGINT, timestamps are TIMESTAMPTZ.
--   * soft delete = deleted_at (only where the legacy systems had it).
--   * legacy_map keeps old ids so an import can be re-run without duplicating rows.

CREATE TABLE users (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username             TEXT        NOT NULL,
    full_name            TEXT        NOT NULL DEFAULT '',
    phone                TEXT        NOT NULL DEFAULT '',
    email                TEXT        NOT NULL DEFAULT '',
    password_hash        TEXT        NOT NULL DEFAULT '',
    -- A hash imported from a legacy system in its own format (scrypt$..., pbkdf2_sha256$...).
    -- Verified once at login, then replaced by a bcrypt password_hash and cleared.
    legacy_hash          TEXT        NOT NULL DEFAULT '',
    must_change_password BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active            BOOLEAN     NOT NULL DEFAULT TRUE,
    is_super             BOOLEAN     NOT NULL DEFAULT FALSE,
    last_login_at        TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_users_username ON users (lower(username)) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_phone ON users (phone) WHERE phone <> '';

CREATE TABLE roles (
    code       TEXT PRIMARY KEY,
    title_fa   TEXT    NOT NULL,
    title_en   TEXT    NOT NULL,
    is_system  BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE role_permissions (
    role_code  TEXT NOT NULL REFERENCES roles(code) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_code, permission)
);

CREATE TABLE user_roles (
    user_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_code TEXT   NOT NULL REFERENCES roles(code) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_code)
);

-- Refresh tokens are opaque and stored hashed; every use rotates them.
CREATE TABLE refresh_tokens (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE,
    client      TEXT        NOT NULL DEFAULT 'web',   -- web | customer-app | technician-app | manager-app
    user_agent  TEXT        NOT NULL DEFAULT '',
    ip          TEXT        NOT NULL DEFAULT '',
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_refresh_user ON refresh_tokens (user_id);

-- Phone + one-time code login, the normal way customers sign in.
CREATE TABLE otp_codes (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    phone      TEXT        NOT NULL,
    code_hash  TEXT        NOT NULL,
    purpose    TEXT        NOT NULL DEFAULT 'login',
    attempts   INT         NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_otp_phone ON otp_codes (phone, created_at DESC);

-- Append-only. Replaces the three separate audit trails of the old systems.
CREATE TABLE audit_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_id    BIGINT,
    actor_name  TEXT        NOT NULL DEFAULT '',
    actor_role  TEXT        NOT NULL DEFAULT '',
    action      TEXT        NOT NULL,
    entity      TEXT        NOT NULL DEFAULT '',
    entity_id   TEXT        NOT NULL DEFAULT '',
    detail      TEXT        NOT NULL DEFAULT '',
    ip          TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX idx_audit_at ON audit_log (at DESC);
CREATE INDEX idx_audit_entity ON audit_log (entity, entity_id);
CREATE INDEX idx_audit_actor ON audit_log (actor_id);

CREATE FUNCTION forbid_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END $$;

CREATE TRIGGER audit_no_update BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION forbid_change();

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE files (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    path        TEXT        NOT NULL,
    original    TEXT        NOT NULL DEFAULT '',
    mime        TEXT        NOT NULL DEFAULT '',
    size_bytes  BIGINT      NOT NULL DEFAULT 0,
    purpose     TEXT        NOT NULL DEFAULT '',
    owner_id    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- SMS patterns (Melli Payamak "base number" bodies) live in the database so a
-- new pattern never needs a redeploy.
CREATE TABLE sms_templates (
    key         TEXT PRIMARY KEY,
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    body_id     INT     NOT NULL DEFAULT 0 CHECK (body_id >= 0),  -- 0 = not configured, do not send
    sample_text TEXT    NOT NULL DEFAULT '',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    is_builtin  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Outbox: handlers enqueue, a background worker delivers and retries, so a
-- provider outage can never fail or slow the request that triggered the SMS.
CREATE TABLE sms_outbox (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    phone        TEXT        NOT NULL,
    template_key TEXT        NOT NULL,
    args         JSONB       NOT NULL DEFAULT '[]',
    status       TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed','skipped')),
    attempts     INT         NOT NULL DEFAULT 0,
    error        TEXT        NOT NULL DEFAULT '',
    next_try_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sms_outbox_due ON sms_outbox (next_try_at) WHERE status = 'pending';

-- In-app notifications for the mobile/web apps.
CREATE TABLE notifications (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT        NOT NULL DEFAULT 'info',
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL DEFAULT '',
    link       TEXT        NOT NULL DEFAULT '',
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notifications_user ON notifications (user_id, created_at DESC);

-- Push tokens for the future mobile apps (FCM/APNs); endpoints exist already.
CREATE TABLE push_tokens (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform   TEXT        NOT NULL,
    token      TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Remembers which legacy row became which new row. Makes imports idempotent.
CREATE TABLE legacy_map (
    source    TEXT   NOT NULL,   -- guarantee | warehouse | attendance
    entity    TEXT   NOT NULL,
    legacy_id TEXT   NOT NULL,
    new_id    BIGINT NOT NULL,
    PRIMARY KEY (source, entity, legacy_id)
);
