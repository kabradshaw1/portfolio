package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.CreateTaskRequest;
import dev.kylebradshaw.task.dto.TaskEventMessage;
import dev.kylebradshaw.task.dto.UpdateTaskRequest;
import dev.kylebradshaw.task.entity.Task;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.TaskRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class TaskService {
    private final TaskRepository taskRepo;
    private final ProjectRepository projectRepo;
    private final UserRepository userRepo;
    private final TaskEventPublisher eventPublisher;

    public TaskService(TaskRepository taskRepo, ProjectRepository projectRepo,
                       UserRepository userRepo, TaskEventPublisher eventPublisher) {
        this.taskRepo = taskRepo;
        this.projectRepo = projectRepo;
        this.userRepo = userRepo;
        this.eventPublisher = eventPublisher;
    }

    @Transactional
    public Task createTask(CreateTaskRequest request, UUID actorId) {
        var project = projectRepo.findById(request.projectId())
                .orElseThrow(() -> new IllegalArgumentException("Project not found"));
        var priority = request.priority() != null ? request.priority()
                : dev.kylebradshaw.task.entity.TaskPriority.MEDIUM;
        var task = new Task(project, request.title(), request.description(), priority, request.dueDate());
        task = taskRepo.save(task);
        eventPublisher.publish("task.created", TaskEventMessage.of(
                "TASK_CREATED", actorId, project.getId(), task.getId(),
                Map.of("task_title", task.getTitle())));
        return task;
    }

    @Transactional
    public Task updateTask(UUID taskId, UpdateTaskRequest request, UUID actorId) {
        Task task = taskRepo.findById(taskId).orElseThrow(() -> new IllegalArgumentException("Task not found"));
        boolean statusChanged = false;
        if (request.title() != null) task.setTitle(request.title());
        if (request.description() != null) task.setDescription(request.description());
        if (request.status() != null && request.status() != task.getStatus()) {
            task.setStatus(request.status());
            statusChanged = true;
        }
        if (request.priority() != null) task.setPriority(request.priority());
        if (request.dueDate() != null) task.setDueDate(request.dueDate());
        task = taskRepo.save(task);
        if (statusChanged) {
            eventPublisher.publish("task.status_changed", TaskEventMessage.of(
                    "STATUS_CHANGED", actorId, task.getProject().getId(), task.getId(),
                    Map.of("task_title", task.getTitle(), "new_status", task.getStatus().name())));
        }
        return task;
    }

    @Transactional
    public Task assignTask(UUID taskId, UUID assigneeId, UUID actorId) {
        Task task = taskRepo.findById(taskId).orElseThrow(() -> new IllegalArgumentException("Task not found"));
        User assignee = userRepo.findById(assigneeId).orElseThrow(() -> new IllegalArgumentException("Assignee not found"));
        task.setAssignee(assignee);
        task = taskRepo.save(task);
        eventPublisher.publish("task.assigned", TaskEventMessage.of(
                "TASK_ASSIGNED", actorId, task.getProject().getId(), task.getId(),
                Map.of("assignee_id", assigneeId.toString(), "task_title", task.getTitle())));
        return task;
    }

    public Task getTask(UUID taskId) {
        return taskRepo.findById(taskId).orElseThrow(() -> new IllegalArgumentException("Task not found"));
    }

    public List<Task> getTasksByProject(UUID projectId) {
        return taskRepo.findByProjectId(projectId);
    }

    @Transactional
    public void deleteTask(UUID taskId, UUID actorId) {
        Task task = taskRepo.findById(taskId).orElseThrow(() -> new IllegalArgumentException("Task not found"));
        eventPublisher.publish("task.deleted", TaskEventMessage.of(
                "TASK_DELETED", actorId, task.getProject().getId(), task.getId(),
                Map.of("task_title", task.getTitle())));
        taskRepo.delete(task);
    }
}
