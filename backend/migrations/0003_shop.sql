-- 0003 shop: addresses, cart, orders, payments, coupons, wishlist.

CREATE TABLE addresses (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT    NOT NULL DEFAULT '',
    recipient   TEXT    NOT NULL,
    phone       TEXT    NOT NULL,
    province    TEXT    NOT NULL DEFAULT '',
    city        TEXT    NOT NULL DEFAULT '',
    address     TEXT    NOT NULL,
    postal_code TEXT    NOT NULL DEFAULT '',
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_addresses_user ON addresses (user_id);

-- A cart belongs to a signed-in user or, for guests, to a random token kept in a cookie/header.
CREATE TABLE carts (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT REFERENCES users(id) ON DELETE CASCADE,
    guest_token   TEXT UNIQUE,
    coupon_code   TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (user_id IS NOT NULL OR guest_token IS NOT NULL)
);
CREATE UNIQUE INDEX uq_carts_user ON carts (user_id) WHERE user_id IS NOT NULL;

CREATE TABLE cart_items (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cart_id    BIGINT NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    product_id BIGINT REFERENCES products(id) ON DELETE CASCADE,
    part_id    BIGINT REFERENCES parts(id) ON DELETE CASCADE,
    qty        INT    NOT NULL CHECK (qty > 0 AND qty <= 99),
    CHECK ((product_id IS NOT NULL)::int + (part_id IS NOT NULL)::int = 1)
);
CREATE UNIQUE INDEX uq_cart_item_product ON cart_items (cart_id, product_id) WHERE product_id IS NOT NULL;
CREATE UNIQUE INDEX uq_cart_item_part    ON cart_items (cart_id, part_id)    WHERE part_id IS NOT NULL;

CREATE TABLE coupons (
    code        TEXT PRIMARY KEY,
    kind        TEXT   NOT NULL CHECK (kind IN ('percent','fixed')),
    value       BIGINT NOT NULL CHECK (value > 0),
    max_discount BIGINT NOT NULL DEFAULT 0,       -- cap for percent coupons, 0 = none
    min_total   BIGINT NOT NULL DEFAULT 0,
    max_uses    INT    NOT NULL DEFAULT 0,        -- 0 = unlimited
    used_count  INT    NOT NULL DEFAULT 0,
    starts_at   TIMESTAMPTZ,
    ends_at     TIMESTAMPTZ,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE SEQUENCE order_no_seq START 100001;

CREATE TABLE orders (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number          TEXT    NOT NULL UNIQUE DEFAULT ('EV' || nextval('order_no_seq')),
    user_id         BIGINT  NOT NULL REFERENCES users(id),
    status          TEXT    NOT NULL DEFAULT 'pending_payment'
                    CHECK (status IN ('pending_payment','paid','processing','shipped','delivered','cancelled','refunded')),
    subtotal        BIGINT  NOT NULL CHECK (subtotal >= 0),
    discount        BIGINT  NOT NULL DEFAULT 0 CHECK (discount >= 0),
    shipping_fee    BIGINT  NOT NULL DEFAULT 0 CHECK (shipping_fee >= 0),
    total           BIGINT  NOT NULL CHECK (total >= 0),
    coupon_code     TEXT    NOT NULL DEFAULT '',
    address         JSONB   NOT NULL,                -- snapshot, later edits never rewrite history
    note            TEXT    NOT NULL DEFAULT '',
    shipping_method TEXT    NOT NULL DEFAULT 'post',
    tracking_code   TEXT    NOT NULL DEFAULT '',
    paid_at         TIMESTAMPTZ,
    shipped_at      TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_orders_user ON orders (user_id, created_at DESC);
CREATE INDEX idx_orders_status ON orders (status);

CREATE TABLE order_items (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id  BIGINT REFERENCES products(id),
    part_id     BIGINT REFERENCES parts(id),
    name        TEXT   NOT NULL,
    unit_price  BIGINT NOT NULL CHECK (unit_price >= 0),
    qty         INT    NOT NULL CHECK (qty > 0),
    total       BIGINT NOT NULL CHECK (total >= 0),
    -- Serial / guarantee code of the physical unit, filled in when it ships.
    serial_code TEXT   NOT NULL DEFAULT ''
);
CREATE INDEX idx_order_items_order ON order_items (order_id);

CREATE TABLE payments (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id     BIGINT NOT NULL REFERENCES orders(id),
    gateway      TEXT   NOT NULL,                 -- zarinpal | mock
    amount       BIGINT NOT NULL CHECK (amount > 0),
    authority    TEXT   NOT NULL DEFAULT '',
    ref_id       TEXT   NOT NULL DEFAULT '',
    card_pan     TEXT   NOT NULL DEFAULT '',
    status       TEXT   NOT NULL DEFAULT 'initiated' CHECK (status IN ('initiated','paid','failed','refunded')),
    raw          JSONB  NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at  TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_payments_authority ON payments (gateway, authority) WHERE authority <> '';
CREATE INDEX idx_payments_order ON payments (order_id);

CREATE TABLE wishlist (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, product_id)
);
