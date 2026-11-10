-- 0002 catalog + warehouse.
-- One catalog serves the shop, the guarantee system and the warehouse:
--   products = finished goods people buy and register guarantees for
--   parts    = spare parts / accessories: stocked in the warehouse, requested
--              by technicians, and (optionally) sold in the shop.

CREATE TABLE categories (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    parent_id  BIGINT REFERENCES categories(id) ON DELETE SET NULL,
    slug       TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL,
    name_en    TEXT    NOT NULL DEFAULT '',
    description TEXT   NOT NULL DEFAULT '',
    sort_order INT     NOT NULL DEFAULT 0,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE products (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    category_id       BIGINT REFERENCES categories(id),
    sku               TEXT    NOT NULL UNIQUE,
    slug              TEXT    NOT NULL UNIQUE,
    name              TEXT    NOT NULL,
    name_en           TEXT    NOT NULL DEFAULT '',
    summary           TEXT    NOT NULL DEFAULT '',
    description       TEXT    NOT NULL DEFAULT '',
    specs             JSONB   NOT NULL DEFAULT '{}',
    images            JSONB   NOT NULL DEFAULT '[]',
    -- The public site currently shows "call for a price"; price_on_request keeps that behaviour.
    price             BIGINT  NOT NULL DEFAULT 0 CHECK (price >= 0),
    compare_price     BIGINT  NOT NULL DEFAULT 0 CHECK (compare_price >= 0),
    price_on_request  BOOLEAN NOT NULL DEFAULT FALSE,
    stock_qty         INT     NOT NULL DEFAULT 0,
    track_stock       BOOLEAN NOT NULL DEFAULT TRUE,
    is_published      BOOLEAN NOT NULL DEFAULT TRUE,
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    -- Guarantee rules (from the guarantee system's products table)
    guarantee_months  INT     NOT NULL DEFAULT 18 CHECK (guarantee_months >= 0),
    golden_months     INT     NOT NULL DEFAULT 3  CHECK (golden_months >= 0),
    code_format       TEXT    NOT NULL DEFAULT 'simple' CHECK (code_format IN ('simple','jalali_encoded','evinki_vacuum','any')),
    code_prefix       TEXT    NOT NULL DEFAULT '',
    code_pattern      TEXT    NOT NULL DEFAULT '',
    sepidar_code      TEXT    NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);
CREATE INDEX idx_products_category ON products (category_id) WHERE deleted_at IS NULL;

CREATE TABLE parts (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code          TEXT    NOT NULL UNIQUE,
    sepidar_code  TEXT    NOT NULL DEFAULT '',
    part_no       TEXT    NOT NULL DEFAULT '',
    name          TEXT    NOT NULL,
    variant       TEXT    NOT NULL DEFAULT '',
    material      TEXT    NOT NULL DEFAULT '',
    weight_g      NUMERIC(10,2),
    unit          TEXT    NOT NULL DEFAULT 'عدد',
    location      TEXT    NOT NULL DEFAULT '',
    min_stock     INT     NOT NULL DEFAULT 0,
    image         TEXT    NOT NULL DEFAULT '',
    note          TEXT    NOT NULL DEFAULT '',
    row_no        INT,
    -- QR labels carry only this token, so editing `code` never invalidates printed labels.
    qr_token      TEXT    NOT NULL UNIQUE,
    -- Prices: what a customer pays in the shop, and what the company pays a
    -- technician for the returned old part. A real price list replaces typing
    -- the price on every invoice.
    sale_price        BIGINT  NOT NULL DEFAULT 0 CHECK (sale_price >= 0),
    buyback_price     BIGINT  NOT NULL DEFAULT 0 CHECK (buyback_price >= 0),
    is_sellable       BOOLEAN NOT NULL DEFAULT FALSE,
    -- Does a repair using this part require the old one back? Most do.
    return_required   BOOLEAN NOT NULL DEFAULT TRUE,
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX idx_parts_name ON parts (name);

-- Which products a part fits (drives "accessories for this vacuum" in the shop
-- and the part picker in the technician app).
CREATE TABLE part_compat (
    part_id    BIGINT NOT NULL REFERENCES parts(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    PRIMARY KEY (part_id, product_id)
);

-- Labour / service items a technician can bill (replaces the old repair_services catalog).
CREATE TABLE service_items (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    fee         BIGINT  NOT NULL DEFAULT 0 CHECK (fee >= 0),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

-- ───────────── warehouse ─────────────
-- Every place stock can sit is a warehouse row. A technician's own stock is
-- simply a warehouse of kind 'technician': parts delivered to them are a
-- transfer into it, parts they use are consumed out of it. That is what makes
-- "my parts" a ledger instead of a flag on each repair line.
CREATE TABLE warehouses (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code                TEXT    NOT NULL UNIQUE,
    title               TEXT    NOT NULL,
    kind                TEXT    NOT NULL CHECK (kind IN ('central','scrap','returns','technician','products')),
    technician_user_id  BIGINT REFERENCES users(id),
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX uq_wh_technician ON warehouses (technician_user_id) WHERE technician_user_id IS NOT NULL;

CREATE TABLE stock_levels (
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id),
    part_id      BIGINT NOT NULL REFERENCES parts(id),
    qty          INT    NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (warehouse_id, part_id)
);

CREATE TABLE warehouse_workers (
    id     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name   TEXT NOT NULL UNIQUE,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE sepidar_batches (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by  BIGINT REFERENCES users(id),
    doc_count   INT NOT NULL,
    line_count  INT NOT NULL,
    from_at     TIMESTAMPTZ,
    to_at       TIMESTAMPTZ,
    group_key   TEXT,
    party       TEXT,
    doc_no      INT,
    doc_date    TEXT
);

CREATE SEQUENCE stock_doc_no_seq START 1;

-- A document groups lines moved together. Rejected/cancelled documents stay
-- (status), and the triggers below stop anyone from deleting history.
CREATE TABLE stock_documents (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    doc_no           BIGINT  NOT NULL UNIQUE DEFAULT nextval('stock_doc_no_seq'),
    direction        TEXT    NOT NULL CHECK (direction IN ('in','out')),
    reason           TEXT    NOT NULL,
    warehouse_id     BIGINT  NOT NULL REFERENCES warehouses(id),
    dest_warehouse_id BIGINT REFERENCES warehouses(id),   -- set for transfers
    worker_id        BIGINT REFERENCES warehouse_workers(id),
    worker_name      TEXT    NOT NULL DEFAULT '',
    note             TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL CHECK (status IN ('pending','approved','rejected','cancelled')),
    -- What produced it: a person at the counter, or another workflow.
    source_type      TEXT    NOT NULL DEFAULT 'manual',  -- manual | part_request | shipment | repair | order | count
    source_id        BIGINT,
    created_by       BIGINT  REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_by       BIGINT  REFERENCES users(id),
    decided_at       TIMESTAMPTZ,
    decision_note    TEXT    NOT NULL DEFAULT '',
    client_ref       TEXT UNIQUE,                    -- idempotency key from the scanner PWA
    sepidar_batch_id BIGINT  REFERENCES sepidar_batches(id),
    deleted_at       TIMESTAMPTZ,
    deleted_by       BIGINT,
    delete_note      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_stock_docs_status ON stock_documents (status);
CREATE INDEX idx_stock_docs_created ON stock_documents (created_at);
CREATE INDEX idx_stock_docs_source ON stock_documents (source_type, source_id);

CREATE TABLE stock_document_lines (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_id  BIGINT NOT NULL REFERENCES stock_documents(id),
    part_id      BIGINT NOT NULL REFERENCES parts(id),
    qty          INT    NOT NULL CHECK (qty > 0),
    note         TEXT   NOT NULL DEFAULT '',
    stock_before INT,
    stock_after  INT
);
CREATE INDEX idx_stock_lines_doc ON stock_document_lines (document_id);
CREATE INDEX idx_stock_lines_part ON stock_document_lines (part_id);

CREATE TRIGGER docs_no_delete BEFORE DELETE ON stock_documents
    FOR EACH ROW EXECUTE FUNCTION forbid_change();
CREATE TRIGGER lines_no_delete BEFORE DELETE ON stock_document_lines
    FOR EACH ROW EXECUTE FUNCTION forbid_change();

-- Stock counts (انبارگردانی)
CREATE TABLE stock_counts (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id),
    title        TEXT   NOT NULL DEFAULT '',
    status       TEXT   NOT NULL DEFAULT 'open' CHECK (status IN ('open','submitted','applied','rejected')),
    created_by   BIGINT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_by BIGINT REFERENCES users(id),
    submitted_at TIMESTAMPTZ,
    decided_by   BIGINT REFERENCES users(id),
    decided_at   TIMESTAMPTZ,
    result       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE stock_count_lines (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    count_id   BIGINT NOT NULL REFERENCES stock_counts(id) ON DELETE CASCADE,
    part_id    BIGINT NOT NULL REFERENCES parts(id),
    counted    INT    NOT NULL CHECK (counted >= 0),
    counted_by BIGINT NOT NULL REFERENCES users(id),
    counted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expected   INT,
    UNIQUE (count_id, part_id)
);
