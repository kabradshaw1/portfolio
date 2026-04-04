package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.Task;
import dev.kylebradshaw.task.entity.TaskPriority;
import dev.kylebradshaw.task.entity.TaskStatus;
import java.time.Instant;
import java.time.LocalDate;
import java.util.UUID;

public record TaskResponse(UUID id, UUID projectId, String title, String description, TaskStatus status,
                           TaskPriority priority, UUID assigneeId, String assigneeName, LocalDate dueDate,
                           Instant createdAt, Instant updatedAt) {
    public static TaskResponse from(Task task) {
        return new TaskResponse(task.getId(), task.getProject().getId(), task.getTitle(), task.getDescription(),
                task.getStatus(), task.getPriority(),
                task.getAssignee() != null ? task.getAssignee().getId() : null,
                task.getAssignee() != null ? task.getAssignee().getName() : null,
                task.getDueDate(), task.getCreatedAt(), task.getUpdatedAt());
    }
}
