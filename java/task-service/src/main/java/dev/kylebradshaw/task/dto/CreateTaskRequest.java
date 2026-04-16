package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.TaskPriority;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;
import java.time.LocalDate;
import java.util.UUID;

public record CreateTaskRequest(@NotNull UUID projectId, @NotBlank @Size(max = 255) String title, @Size(max = 5000) String description, TaskPriority priority, LocalDate dueDate) {}
