CREATE TABLE processed_projection_events (
    projection_name TEXT NOT NULL,
    event_id        UUID NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (projection_name, event_id)
);

CREATE INDEX idx_processed_projection_events_processed_at
    ON processed_projection_events(processed_at);
