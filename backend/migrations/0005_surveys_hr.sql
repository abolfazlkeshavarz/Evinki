-- 0005 surveys (the old "polls") + HR / attendance.

-- ───────────── surveys ─────────────
-- Simplification: a survey is a template + a trigger. "After every completed
-- repair / delivered order, ask these questions" runs on its own; manual
-- campaigns with filters (the old poll flow) still work.
CREATE TABLE surveys (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title           TEXT   NOT NULL,
    description     TEXT   NOT NULL DEFAULT '',
    status          TEXT   NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','closed')),
    trigger         TEXT   NOT NULL DEFAULT 'manual' CHECK (trigger IN ('manual','repair_approved','order_delivered')),
    sms_template    TEXT   NOT NULL DEFAULT 'survey_invite',
    offer_template  TEXT   NOT NULL DEFAULT '',
    link_mode       TEXT   NOT NULL DEFAULT 'url' CHECK (link_mode IN ('url','token')),
    filter          JSONB  NOT NULL DEFAULT '{}',   -- city, province, product_id, purchased_before, expiry_before
    created_by      BIGINT REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at       TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ
);

CREATE TABLE survey_questions (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    survey_id  BIGINT NOT NULL REFERENCES surveys(id) ON DELETE CASCADE,
    position   INT    NOT NULL DEFAULT 0,
    text       TEXT   NOT NULL,
    kind       TEXT   NOT NULL DEFAULT 'rating' CHECK (kind IN ('rating','nps','yes_no','choice','text')),
    options    JSONB  NOT NULL DEFAULT '[]',
    required   BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_survey_q ON survey_questions (survey_id, position);

CREATE TABLE survey_recipients (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    survey_id      BIGINT NOT NULL REFERENCES surveys(id) ON DELETE CASCADE,
    customer_id    BIGINT REFERENCES customers(id) ON DELETE SET NULL,
    guarantee_id   BIGINT REFERENCES guarantees(id) ON DELETE SET NULL,
    repair_id      BIGINT REFERENCES repairs(id) ON DELETE SET NULL,
    order_id       BIGINT REFERENCES orders(id) ON DELETE SET NULL,
    technician_id  BIGINT REFERENCES technicians(user_id) ON DELETE SET NULL,
    token          TEXT   NOT NULL UNIQUE,
    customer_name  TEXT   NOT NULL DEFAULT '',
    phone          TEXT   NOT NULL DEFAULT '',
    city           TEXT   NOT NULL DEFAULT '',
    product_name   TEXT   NOT NULL DEFAULT '',
    sms_status     TEXT   NOT NULL DEFAULT 'pending' CHECK (sms_status IN ('pending','sent','failed','skipped')),
    sms_error      TEXT   NOT NULL DEFAULT '',
    sent_at        TIMESTAMPTZ,
    opened_at      TIMESTAMPTZ,
    responded_at   TIMESTAMPTZ,
    offer_sent_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_survey_recipients_survey ON survey_recipients (survey_id);
-- A customer is invited at most once per repair/order per survey.
CREATE UNIQUE INDEX uq_survey_repair ON survey_recipients (survey_id, repair_id) WHERE repair_id IS NOT NULL;
CREATE UNIQUE INDEX uq_survey_order  ON survey_recipients (survey_id, order_id)  WHERE order_id  IS NOT NULL;

CREATE TABLE survey_answers (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recipient_id  BIGINT NOT NULL REFERENCES survey_recipients(id) ON DELETE CASCADE,
    question_id   BIGINT NOT NULL REFERENCES survey_questions(id) ON DELETE CASCADE,
    answer_text   TEXT   NOT NULL DEFAULT '',
    answer_number INT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recipient_id, question_id)
);

-- ───────────── HR / attendance ─────────────
CREATE TABLE departments (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE shifts (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    start_time        TIME NOT NULL,
    end_time          TIME NOT NULL,
    crosses_midnight  BOOLEAN NOT NULL DEFAULT FALSE,
    grace_in_minutes  INT NOT NULL DEFAULT 10,
    grace_out_minutes INT NOT NULL DEFAULT 10,
    -- Iranian weekday numbers: Saturday=0 ... Friday=6
    work_days         TEXT NOT NULL DEFAULT '0,1,2,3,4,5',
    break_minutes     INT NOT NULL DEFAULT 0,
    is_active         BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE holidays (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    day         DATE NOT NULL UNIQUE,
    title       TEXT NOT NULL,
    is_official BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE employees (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    personnel_code TEXT NOT NULL UNIQUE,
    first_name     TEXT NOT NULL,
    last_name      TEXT NOT NULL,
    national_code  TEXT UNIQUE,
    mobile         TEXT NOT NULL DEFAULT '',
    position       TEXT NOT NULL DEFAULT '',
    department_id  BIGINT REFERENCES departments(id) ON DELETE SET NULL,
    shift_id       BIGINT REFERENCES shifts(id) ON DELETE SET NULL,
    hire_date      DATE,
    pin_hash       TEXT NOT NULL DEFAULT '',
    photo          TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE face_embeddings (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id BIGINT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    vector      TEXT   NOT NULL,                   -- JSON array; model output is NOT normalised on purpose
    dim         INT    NOT NULL,
    model_name  TEXT   NOT NULL DEFAULT 'face-api-128',
    quality     REAL,
    image       TEXT   NOT NULL DEFAULT '',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_face_emp ON face_embeddings (employee_id);

-- Kiosk tablets and fingerprint readers. They authenticate with an API key, not a user login.
CREATE TABLE attendance_devices (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL DEFAULT 'tablet' CHECK (kind IN ('tablet','fingerprint')),
    device_uid      TEXT NOT NULL UNIQUE,
    api_key_hash    TEXT NOT NULL,
    location        TEXT NOT NULL DEFAULT '',
    last_seen_at    TIMESTAMPTZ,
    last_sync_at    TIMESTAMPTZ,
    app_version     TEXT NOT NULL DEFAULT '',
    pending_count   INT  NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    last_scan       JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_att_devices_key ON attendance_devices (api_key_hash);

CREATE TABLE attendance_records (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id   BIGINT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    device_id     BIGINT REFERENCES attendance_devices(id) ON DELETE SET NULL,
    kind          TEXT   NOT NULL CHECK (kind IN ('in','out')),
    method        TEXT   NOT NULL DEFAULT 'face' CHECK (method IN ('face','fingerprint','pin','manual','admin_fix')),
    happened_at   TIMESTAMPTZ NOT NULL,
    work_date     DATE   NOT NULL,
    confidence    REAL,
    snapshot      TEXT   NOT NULL DEFAULT '',
    -- Kiosks make this offline and re-send batches; the unique key makes re-sending safe.
    client_uuid   TEXT UNIQUE,
    created_offline BOOLEAN NOT NULL DEFAULT FALSE,
    synced_at     TIMESTAMPTZ,
    note          TEXT NOT NULL DEFAULT '',
    created_by    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_att_emp_day ON attendance_records (employee_id, work_date);
CREATE INDEX idx_att_happened ON attendance_records (happened_at);

CREATE TABLE leave_requests (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id   BIGINT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    leave_type    TEXT NOT NULL DEFAULT 'daily' CHECK (leave_type IN ('daily','hourly','sick','emergency','mission','unpaid')),
    start_at      TIMESTAMPTZ NOT NULL,
    end_at        TIMESTAMPTZ NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    reason        TEXT NOT NULL DEFAULT '',
    review_note   TEXT NOT NULL DEFAULT '',
    reviewed_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (end_at >= start_at)
);
CREATE INDEX idx_leave_emp ON leave_requests (employee_id, start_at, end_at);

CREATE TABLE tasks (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title             TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    employee_id       BIGINT REFERENCES employees(id) ON DELETE CASCADE,
    department_id     BIGINT REFERENCES departments(id) ON DELETE SET NULL,
    assigned_by       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    status            TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','in_progress','done','cancelled')),
    priority          TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','urgent')),
    recurrence        TEXT NOT NULL DEFAULT 'none' CHECK (recurrence IN ('none','daily','weekly','monthly')),
    due_date          DATE,
    start_date        DATE,
    end_date          DATE,
    estimated_minutes INT,
    progress          INT NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    completed_at      TIMESTAMPTZ,
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tasks_emp ON tasks (employee_id, status);

CREATE TABLE task_logs (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id       BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    log_date      DATE   NOT NULL,
    status        TEXT   NOT NULL DEFAULT 'done',
    spent_minutes INT,
    note          TEXT   NOT NULL DEFAULT '',
    recorded_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (task_id, log_date)
);

CREATE TABLE fingerprint_templates (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id   BIGINT NOT NULL UNIQUE REFERENCES employees(id) ON DELETE CASCADE,
    template_data BYTEA  NOT NULL,
    model_name    TEXT   NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE fingerprint_slots (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    device_id   BIGINT NOT NULL REFERENCES attendance_devices(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    slot_id     INT    NOT NULL,
    synced_at   TIMESTAMPTZ,
    UNIQUE (device_id, slot_id),
    UNIQUE (device_id, employee_id)
);

CREATE TABLE fingerprint_enroll_jobs (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id  BIGINT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    device_id    BIGINT NOT NULL REFERENCES attendance_devices(id) ON DELETE CASCADE,
    status       TEXT   NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','done','failed','cancelled')),
    error        TEXT   NOT NULL DEFAULT '',
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
