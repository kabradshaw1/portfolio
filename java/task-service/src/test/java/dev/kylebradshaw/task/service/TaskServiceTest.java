package dev.kylebradshaw.task.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.task.dto.CreateTaskRequest;
import dev.kylebradshaw.task.dto.TaskEventMessage;
import dev.kylebradshaw.task.dto.UpdateTaskRequest;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.Task;
import dev.kylebradshaw.task.entity.TaskPriority;
import dev.kylebradshaw.task.entity.TaskStatus;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.TaskRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class TaskServiceTest {

    @Mock private TaskRepository taskRepo;
    @Mock private ProjectRepository projectRepo;
    @Mock private UserRepository userRepo;
    @Mock private TaskEventPublisher eventPublisher;

    private TaskService service;

    @BeforeEach
    void setUp() {
        service = new TaskService(taskRepo, projectRepo, userRepo, eventPublisher);
    }

    @Test
    void createTask_savesAndPublishesEvent() {
        UUID userId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        UUID projectId = UUID.randomUUID();

        when(projectRepo.findById(projectId)).thenReturn(Optional.of(project));
        when(taskRepo.save(any(Task.class))).thenAnswer(inv -> inv.getArgument(0));

        var request = new CreateTaskRequest(projectId, "Fix bug", "Fix the login bug", TaskPriority.HIGH, null);
        Task result = service.createTask(request, userId);

        assertThat(result.getTitle()).isEqualTo("Fix bug");
        assertThat(result.getPriority()).isEqualTo(TaskPriority.HIGH);
        verify(eventPublisher).publish(eq("task.created"), any(TaskEventMessage.class));
    }

    @Test
    void updateTask_changesFieldsAndPublishesStatusEvent() {
        UUID userId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Old title", "Old desc", TaskPriority.LOW, null);
        UUID taskId = UUID.randomUUID();

        when(taskRepo.findById(taskId)).thenReturn(Optional.of(task));
        when(taskRepo.save(any(Task.class))).thenAnswer(inv -> inv.getArgument(0));

        var request = new UpdateTaskRequest("New title", null, TaskStatus.IN_PROGRESS, null, null);
        Task result = service.updateTask(taskId, request, userId);

        assertThat(result.getTitle()).isEqualTo("New title");
        assertThat(result.getStatus()).isEqualTo(TaskStatus.IN_PROGRESS);
        verify(eventPublisher).publish(eq("task.status_changed"), any(TaskEventMessage.class));
    }

    @Test
    void assignTask_setsAssigneeAndPublishesEvent() {
        UUID userId = UUID.randomUUID();
        UUID assigneeId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        User assignee = new User("dev@example.com", "Developer", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Task", "Desc", TaskPriority.MEDIUM, null);
        UUID taskId = UUID.randomUUID();

        when(taskRepo.findById(taskId)).thenReturn(Optional.of(task));
        when(userRepo.findById(assigneeId)).thenReturn(Optional.of(assignee));
        when(taskRepo.save(any(Task.class))).thenAnswer(inv -> inv.getArgument(0));

        Task result = service.assignTask(taskId, assigneeId, userId);
        assertThat(result.getAssignee()).isEqualTo(assignee);
        verify(eventPublisher).publish(eq("task.assigned"), any(TaskEventMessage.class));
    }

    @Test
    void getTasksByProject_returnsAll() {
        UUID projectId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Task", null, TaskPriority.LOW, null);
        when(taskRepo.findByProjectId(projectId)).thenReturn(List.of(task));

        List<Task> result = service.getTasksByProject(projectId);
        assertThat(result).hasSize(1);
    }
}
