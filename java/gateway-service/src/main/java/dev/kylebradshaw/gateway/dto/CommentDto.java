package dev.kylebradshaw.gateway.dto;

public record CommentDto(String id, String taskId, String authorId, String body, String createdAt) {}
