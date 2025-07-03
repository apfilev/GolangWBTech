BEGIN;

-- Таблица заказов
CREATE TABLE "order" (
    order_uid VARCHAR PRIMARY KEY,
    track_number VARCHAR NOT NULL,
    entry VARCHAR NOT NULL,
    locale VARCHAR NOT NULL,
    internal_signature VARCHAR NOT NULL,
    customer_id VARCHAR NOT NULL,
    delivery_service VARCHAR NOT NULL,
    shardkey VARCHAR NOT NULL,
    sm_id INTEGER NOT NULL,
    date_created TIMESTAMP WITH TIME ZONE NOT NULL,
    oof_shard VARCHAR NOT NULL,
    delivery JSONB NOT NULL
);

-- Таблица платежей
CREATE TABLE payment (
    transaction VARCHAR PRIMARY KEY,
    request_id VARCHAR NOT NULL,
    currency VARCHAR NOT NULL,
    provider VARCHAR NOT NULL,
    amount INTEGER NOT NULL,
    payment_dt INTEGER NOT NULL,
    bank VARCHAR NOT NULL,
    delivery_cost INTEGER NOT NULL,
    goods_total INTEGER NOT NULL,
    custom_fee INTEGER NOT NULL,
    order_uid VARCHAR NOT NULL,
    FOREIGN KEY (order_uid) REFERENCES "order" (order_uid)
);

-- Таблица товаров
CREATE TABLE item (
    chrt_id BIGSERIAL PRIMARY KEY,
    track_number VARCHAR NOT NULL,
    price INTEGER NOT NULL,
    rid VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    sale INTEGER NOT NULL,
    size VARCHAR NOT NULL,
    total_price INTEGER NOT NULL,
    nm_id BIGINT NOT NULL,
    brand VARCHAR NOT NULL,
    status INTEGER NOT NULL,
    order_uid VARCHAR NOT NULL,
    FOREIGN KEY (order_uid) REFERENCES "order" (order_uid)
);

COMMIT; 