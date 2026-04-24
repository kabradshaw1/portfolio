CREATE TABLE order_timeline (
    event_id      UUID PRIMARY KEY,
    order_id      UUID NOT NULL,
    event_type    TEXT NOT NULL,
    event_version INT NOT NULL DEFAULT 1,
    data_json     JSONB NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_timeline_order_id ON order_timeline(order_id, timestamp);

CREATE TABLE order_summary (
    order_id       UUID PRIMARY KEY,
    user_id        UUID NOT NULL,
    status         TEXT NOT NULL DEFAULT 'created',
    total_cents    BIGINT NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL DEFAULT 'USD',
    items_json     JSONB,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    completed_at   TIMESTAMPTZ,
    failure_reason TEXT
);
CREATE INDEX idx_summary_user_id ON order_summary(user_id);
CREATE INDEX idx_summary_status ON order_summary(status);

CREATE TABLE order_stats (
    hour_bucket           TIMESTAMPTZ PRIMARY KEY,
    orders_created        INT DEFAULT 0,
    orders_completed      INT DEFAULT 0,
    orders_failed         INT DEFAULT 0,
    avg_completion_seconds FLOAT DEFAULT 0,
    total_revenue_cents   BIGINT DEFAULT 0
);

CREATE TABLE replay_status (
    id                SERIAL PRIMARY KEY,
    is_replaying      BOOLEAN DEFAULT FALSE,
    projection        TEXT NOT NULL DEFAULT 'all',
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    events_processed  BIGINT DEFAULT 0,
    total_events      BIGINT DEFAULT 0
);
