package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.TaskPriority;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import java.time.LocalDate;
import java.util.UUID;

public record CreateTaskRequest(@NotNull UUID projectId, @NotBlank String title, String description, TaskPriority priority, LocalDate dueDate) {}
