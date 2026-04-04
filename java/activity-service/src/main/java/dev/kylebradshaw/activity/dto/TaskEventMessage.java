package dev.kylebradshaw.activity.dto;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

public record TaskEventMessage(UUID eventId, String eventType, Instant timestamp, UUID actorId,
                                UUID projectId, UUID taskId, Map<String, Object> data) {}
