package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.TaskPriority;
import dev.kylebradshaw.task.entity.TaskStatus;
import java.time.LocalDate;

public record UpdateTaskRequest(String title, String description, TaskStatus status, TaskPriority priority, LocalDate dueDate) {}
