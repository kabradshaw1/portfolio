package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.CreateTaskRequest;
import dev.kylebradshaw.task.dto.TaskResponse;
import dev.kylebradshaw.task.dto.UpdateTaskRequest;
import dev.kylebradshaw.task.service.TaskService;
import jakarta.validation.Valid;
import java.util.List;
import java.util.UUID;
import org.springframework.http.HttpStatus;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/tasks")
public class TaskController {

    private final TaskService taskService;

    public TaskController(TaskService taskService) {
        this.taskService = taskService;
    }

    private UUID getAuthenticatedUserId() {
        var auth = SecurityContextHolder.getContext().getAuthentication();
        if (auth == null) {
            throw new IllegalStateException("No authenticated user");
        }
        return UUID.fromString(auth.getName());
    }

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public TaskResponse createTask(@Valid @RequestBody CreateTaskRequest request) {
        UUID actorId = getAuthenticatedUserId();
        return TaskResponse.from(taskService.createTask(request, actorId));
    }

    @GetMapping
    public List<TaskResponse> getTasksByProject(@RequestParam UUID projectId) {
        return taskService.getTasksByProject(projectId).stream()
                .map(TaskResponse::from)
                .toList();
    }

    @GetMapping("/{id}")
    public TaskResponse getTask(@PathVariable UUID id) {
        return TaskResponse.from(taskService.getTask(id));
    }

    @PutMapping("/{id}")
    public TaskResponse updateTask(
            @PathVariable UUID id,
            @RequestBody UpdateTaskRequest request) {
        UUID actorId = getAuthenticatedUserId();
        return TaskResponse.from(taskService.updateTask(id, request, actorId));
    }

    @PutMapping("/{id}/assign/{assigneeId}")
    public TaskResponse assignTask(
            @PathVariable UUID id,
            @PathVariable UUID assigneeId) {
        UUID actorId = getAuthenticatedUserId();
        return TaskResponse.from(taskService.assignTask(id, assigneeId, actorId));
    }

    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void deleteTask(@PathVariable UUID id) {
        UUID actorId = getAuthenticatedUserId();
        taskService.deleteTask(id, actorId);
    }
}
