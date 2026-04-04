package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.Project;
import java.time.Instant;
import java.util.UUID;

public record ProjectResponse(UUID id, String name, String description, UUID ownerId, String ownerName, Instant createdAt) {
    public static ProjectResponse from(Project project) {
        return new ProjectResponse(project.getId(), project.getName(), project.getDescription(),
                project.getOwner().getId(), project.getOwner().getName(), project.getCreatedAt());
    }
}
