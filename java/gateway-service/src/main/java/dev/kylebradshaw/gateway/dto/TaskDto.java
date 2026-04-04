package dev.kylebradshaw.gateway.dto;

public record TaskDto(String id, String projectId, String title, String description, String status, String priority,
                      String assigneeId, String assigneeName, String dueDate, String createdAt, String updatedAt) {}
