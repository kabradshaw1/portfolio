CREATE TABLE processed_events (
    stripe_event_id TEXT PRIMARY KEY,
    event_type      TEXT NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
