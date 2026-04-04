package dev.kylebradshaw.task.dto;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

public record TaskEventMessage(UUID eventId, String eventType, Instant timestamp, UUID actorId,
                                UUID projectId, UUID taskId, Map<String, Object> data) {
    public static TaskEventMessage of(String eventType, UUID actorId, UUID projectId, UUID taskId, Map<String, Object> data) {
        return new TaskEventMessage(UUID.randomUUID(), eventType, Instant.now(), actorId, projectId, taskId, data);
    }
}
