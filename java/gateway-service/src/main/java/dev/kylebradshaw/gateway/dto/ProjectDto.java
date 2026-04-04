package dev.kylebradshaw.gateway.dto;

public record ProjectDto(String id, String name, String description, String ownerId, String ownerName, String createdAt) {}
