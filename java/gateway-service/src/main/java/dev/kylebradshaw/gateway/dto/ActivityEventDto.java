package dev.kylebradshaw.gateway.dto;

public record ActivityEventDto(String id, String projectId, String taskId, String actorId, String eventType,
                                String metadata, String timestamp) {}
