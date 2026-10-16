CREATE TABLE products (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sale_starts_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE ticket_types (
    id TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id),
    name TEXT NOT NULL,
    price_cents BIGINT NOT NULL CHECK(price_cents>0),
    available BIGINT NOT NULL CHECK(available>=0),
    reserved BIGINT NOT NULL DEFAULT 0 CHECK(reserved>=0),
    sold BIGINT NOT NULL DEFAULT 0 CHECK(sold>=0),
    withheld BIGINT NOT NULL DEFAULT 0 CHECK(withheld>=0),
    UNIQUE(product_id,id)
);

CREATE TABLE purchase_claims (
    product_id TEXT NOT NULL REFERENCES products(id),
    buyer_id BIGINT NOT NULL CHECK(buyer_id>0),
    order_id TEXT,
    permanent BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY(product_id,buyer_id)
);

CREATE TABLE orders (
    id TEXT PRIMARY KEY,
    product_id TEXT NOT NULL,
    ticket_type_id TEXT NOT NULL,
    buyer_id BIGINT NOT NULL CHECK(buyer_id>0),
    request_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('pending','paid','closed','refunding','refunded')),
    price_cents BIGINT NOT NULL CHECK(price_cents>0),
    product_name TEXT NOT NULL,
    ticket_type_name TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    payment_state TEXT NOT NULL DEFAULT 'unpaid' CHECK(payment_state IN ('unpaid','processing','paid','refund_pending','refunded')),
    payment_reference TEXT UNIQUE,
    reconciled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(buyer_id,request_id),
    FOREIGN KEY(product_id,ticket_type_id) REFERENCES ticket_types(product_id,id)
);

CREATE UNIQUE INDEX orders_active_purchase ON orders(product_id,buyer_id)
    WHERE state IN ('pending','paid','refunding');

CREATE INDEX orders_expiry ON orders(expires_at)
    WHERE state='pending';

CREATE INDEX orders_refunds ON orders(created_at)
    WHERE payment_state='refund_pending';

CREATE TABLE inventory_events (
    id BIGSERIAL PRIMARY KEY,
    ticket_type_id TEXT NOT NULL REFERENCES ticket_types(id),
    operation_id TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL,
    available_delta BIGINT NOT NULL,
    reserved_delta BIGINT NOT NULL,
    sold_delta BIGINT NOT NULL,
    withheld_delta BIGINT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE simulated_payments (
    order_id TEXT PRIMARY KEY,
    reference TEXT NOT NULL UNIQUE,
    amount BIGINT NOT NULL,
    refunded BOOLEAN NOT NULL DEFAULT false,
    closed BOOLEAN NOT NULL DEFAULT false
);
