-- 0004 warranty + after-sales service.
--
-- Simplification over the old guarantee system: it kept a part request line, a
-- repair line, a shipment line and a price on each of them, and the technician
-- had to follow one physical part across all four. Here a single row per used
-- part (repair_parts) carries the whole life of that part: where the new one
-- came from, whether the old one is owed back, which parcel it went in, whether
-- it arrived, and what it is worth. Shipments and invoices are just views over
-- those rows.

CREATE TABLE customers (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    full_name   TEXT NOT NULL,
    phone       TEXT NOT NULL DEFAULT '',
    national_id TEXT NOT NULL DEFAULT '',
    province    TEXT NOT NULL DEFAULT '',
    city        TEXT NOT NULL DEFAULT '',
    address     TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX idx_customers_phone ON customers (phone);
CREATE INDEX idx_customers_user ON customers (user_id);
CREATE INDEX idx_customers_national ON customers (national_id);

CREATE TABLE guarantees (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code                 TEXT   NOT NULL,
    customer_id          BIGINT NOT NULL REFERENCES customers(id),
    product_id           BIGINT NOT NULL REFERENCES products(id),
    purchase_date        DATE   NOT NULL,
    expiry_date          DATE   NOT NULL,
    golden_start_date    DATE,
    golden_expiry_date   DATE,
    status               TEXT   NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending','approved','rejected','renewed','cancelled','expired')),
    source               TEXT   NOT NULL DEFAULT 'admin',   -- web | admin | order | import | app
    order_item_id        BIGINT REFERENCES order_items(id),
    invoice_file         TEXT   NOT NULL DEFAULT '',
    card_file            TEXT   NOT NULL DEFAULT '',
    notes                TEXT   NOT NULL DEFAULT '',
    created_by           BIGINT REFERENCES users(id),
    approved_by          BIGINT REFERENCES users(id),
    approved_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ,
    CHECK (expiry_date >= purchase_date)
);
-- A code can be re-registered after a rejection/cancellation, never while it is live.
CREATE UNIQUE INDEX uq_guarantees_live_code ON guarantees (code)
    WHERE deleted_at IS NULL AND status IN ('pending','approved','renewed','expired');
CREATE INDEX idx_guarantees_customer ON guarantees (customer_id);
CREATE INDEX idx_guarantees_product ON guarantees (product_id);
CREATE INDEX idx_guarantees_expiry ON guarantees (expiry_date);

CREATE TABLE technicians (
    user_id       BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    national_id   TEXT NOT NULL DEFAULT '',
    province      TEXT NOT NULL DEFAULT '',
    city          TEXT NOT NULL DEFAULT '',
    address       TEXT NOT NULL DEFAULT '',
    about         TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'approved' CHECK (status IN ('pending','approved','rejected')),
    applied_at    TIMESTAMPTZ,
    reviewed_at   TIMESTAMPTZ,
    reviewed_by   BIGINT REFERENCES users(id),
    review_notes  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The technician's account: every credit (old parts bought back, labour) and
-- every payment is a signed row, so "how much am I owed" is a SUM, not a report.
CREATE TABLE technician_ledger (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    technician_id  BIGINT NOT NULL REFERENCES technicians(user_id),
    kind           TEXT   NOT NULL CHECK (kind IN ('part_buyback','labor','adjustment','payment')),
    amount         BIGINT NOT NULL,                  -- + the company owes, - the company paid
    ref_type       TEXT   NOT NULL DEFAULT '',
    ref_id         BIGINT,
    note           TEXT   NOT NULL DEFAULT '',
    created_by     BIGINT REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ledger_tech ON technician_ledger (technician_id, created_at DESC);
CREATE TRIGGER ledger_no_change BEFORE UPDATE OR DELETE ON technician_ledger
    FOR EACH ROW EXECUTE FUNCTION forbid_change();

CREATE SEQUENCE repair_no_seq START 1001;

CREATE TABLE repairs (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number                TEXT   NOT NULL UNIQUE DEFAULT ('SR' || nextval('repair_no_seq')),
    guarantee_id          BIGINT REFERENCES guarantees(id),
    customer_id           BIGINT NOT NULL REFERENCES customers(id),
    product_id            BIGINT REFERENCES products(id),
    technician_id         BIGINT REFERENCES technicians(user_id),
    -- requested  : customer (or office) opened a service request, nobody assigned
    -- assigned   : a technician owns it
    -- in_progress: technician started work
    -- waiting_parts: blocked on a part request
    -- submitted  : technician filed the report, waiting for the office
    -- approved / rejected: office decision
    -- cancelled
    status                TEXT   NOT NULL DEFAULT 'assigned' CHECK (status IN
                          ('requested','assigned','in_progress','waiting_parts','submitted','approved','rejected','cancelled')),
    source                TEXT   NOT NULL DEFAULT 'technician',   -- technician | customer | office
    complaint             TEXT   NOT NULL DEFAULT '',
    diagnosis             TEXT   NOT NULL DEFAULT '',
    report                TEXT   NOT NULL DEFAULT '',
    -- Stamped at creation, so renewing the guarantee later can't rewrite what was true then.
    guarantee_was_expired BOOLEAN NOT NULL DEFAULT FALSE,
    -- Out-of-warranty work is billed to the customer.
    customer_charge       BIGINT NOT NULL DEFAULT 0 CHECK (customer_charge >= 0),
    customer_paid         BOOLEAN NOT NULL DEFAULT FALSE,
    visit_at              TIMESTAMPTZ,
    started_at            TIMESTAMPTZ,
    submitted_at          TIMESTAMPTZ,
    completed_at          TIMESTAMPTZ,
    reviewed_by           BIGINT REFERENCES users(id),
    reviewed_by_technician BIGINT REFERENCES technicians(user_id),
    reviewed_at           TIMESTAMPTZ,
    review_notes          TEXT   NOT NULL DEFAULT '',
    created_by            BIGINT REFERENCES users(id),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ
);
CREATE INDEX idx_repairs_tech ON repairs (technician_id, status);
CREATE INDEX idx_repairs_guarantee ON repairs (guarantee_id);
CREATE INDEX idx_repairs_status ON repairs (status);

CREATE SEQUENCE part_request_no_seq START 1001;

CREATE TABLE part_requests (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number                TEXT   NOT NULL UNIQUE DEFAULT ('PR' || nextval('part_request_no_seq')),
    technician_id         BIGINT NOT NULL REFERENCES technicians(user_id),
    repair_id             BIGINT REFERENCES repairs(id) ON DELETE SET NULL,
    status                TEXT   NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','approved','delivered','not_delivered','cancelled')),
    notes                 TEXT   NOT NULL DEFAULT '',
    guarantee_was_expired BOOLEAN NOT NULL DEFAULT FALSE,
    reviewed_by           BIGINT REFERENCES users(id),
    reviewed_at           TIMESTAMPTZ,
    review_notes          TEXT   NOT NULL DEFAULT '',
    delivered_at          TIMESTAMPTZ,
    stock_document_id     BIGINT REFERENCES stock_documents(id),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ
);
CREATE INDEX idx_part_requests_tech ON part_requests (technician_id, status);
CREATE INDEX idx_part_requests_repair ON part_requests (repair_id);

CREATE TABLE part_request_items (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    request_id   BIGINT NOT NULL REFERENCES part_requests(id) ON DELETE CASCADE,
    kind         TEXT   NOT NULL CHECK (kind IN ('part','service','custom')),
    part_id      BIGINT REFERENCES parts(id),
    service_id   BIGINT REFERENCES service_items(id),
    custom_name  TEXT   NOT NULL DEFAULT '',
    qty          INT    NOT NULL DEFAULT 1 CHECK (qty BETWEEN 1 AND 999),
    CHECK ((kind = 'part' AND part_id IS NOT NULL)
        OR (kind = 'service' AND service_id IS NOT NULL)
        OR (kind = 'custom' AND custom_name <> ''))
);
CREATE INDEX idx_pri_request ON part_request_items (request_id);

CREATE SEQUENCE shipment_no_seq START 1001;

CREATE TABLE shipments (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number          TEXT   NOT NULL UNIQUE DEFAULT ('SH' || nextval('shipment_no_seq')),
    technician_id   BIGINT NOT NULL REFERENCES technicians(user_id),
    status          TEXT   NOT NULL DEFAULT 'sent'
                    CHECK (status IN ('sent','received','invoiced','paid','rejected','cancelled')),
    method          TEXT   NOT NULL CHECK (method IN ('post','courier','in_person','other')),
    tracking_code   TEXT   NOT NULL DEFAULT '',
    sent_on         DATE   NOT NULL DEFAULT CURRENT_DATE,
    notes           TEXT   NOT NULL DEFAULT '',
    received_at     TIMESTAMPTZ,
    received_by     BIGINT REFERENCES users(id),
    receive_notes   TEXT   NOT NULL DEFAULT '',
    invoiced_at     TIMESTAMPTZ,
    invoiced_by     BIGINT REFERENCES users(id),
    invoice_total   BIGINT NOT NULL DEFAULT 0 CHECK (invoice_total >= 0),
    paid_at         TIMESTAMPTZ,
    paid_by         BIGINT REFERENCES users(id),
    payment_ref     TEXT   NOT NULL DEFAULT '',
    payment_notes   TEXT   NOT NULL DEFAULT '',
    review_notes    TEXT   NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);
CREATE INDEX idx_shipments_tech ON shipments (technician_id, status);

-- One row per part used on a repair. See the note at the top of this file.
CREATE TABLE repair_parts (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    repair_id       BIGINT NOT NULL REFERENCES repairs(id) ON DELETE CASCADE,
    part_id         BIGINT REFERENCES parts(id),
    custom_name     TEXT   NOT NULL DEFAULT '',
    qty             INT    NOT NULL DEFAULT 1 CHECK (qty BETWEEN 1 AND 999),
    report          TEXT   NOT NULL DEFAULT '',
    -- Where the new part came from. 'stock' means it was taken from the
    -- technician's own warehouse, which is debited when the report is submitted.
    source          TEXT   NOT NULL DEFAULT 'stock' CHECK (source IN ('stock','custom')),
    request_item_id BIGINT REFERENCES part_request_items(id) ON DELETE SET NULL,
    -- The old part.
    return_required BOOLEAN NOT NULL DEFAULT TRUE,
    return_state    TEXT   NOT NULL DEFAULT 'pending'
                    CHECK (return_state IN ('none','pending','in_shipment','received','not_received')),
    shipment_id     BIGINT REFERENCES shipments(id) ON DELETE SET NULL,
    condition_note  TEXT   NOT NULL DEFAULT '',
    buyback_price   BIGINT NOT NULL DEFAULT 0 CHECK (buyback_price >= 0),
    stock_document_id BIGINT REFERENCES stock_documents(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (part_id IS NOT NULL OR custom_name <> '')
);
CREATE INDEX idx_repair_parts_repair ON repair_parts (repair_id);
CREATE INDEX idx_repair_parts_ship ON repair_parts (shipment_id);
CREATE INDEX idx_repair_parts_return ON repair_parts (return_state);

CREATE TABLE repair_services (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    repair_id   BIGINT NOT NULL REFERENCES repairs(id) ON DELETE CASCADE,
    service_id  BIGINT NOT NULL REFERENCES service_items(id),
    qty         INT    NOT NULL DEFAULT 1 CHECK (qty BETWEEN 1 AND 99),
    fee         BIGINT NOT NULL DEFAULT 0 CHECK (fee >= 0),
    report      TEXT   NOT NULL DEFAULT ''
);
CREATE INDEX idx_repair_services_repair ON repair_services (repair_id);

CREATE TABLE repair_photos (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    repair_id  BIGINT NOT NULL REFERENCES repairs(id) ON DELETE CASCADE,
    file       TEXT   NOT NULL,
    caption    TEXT   NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
