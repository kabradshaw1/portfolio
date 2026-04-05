package dev.kylebradshaw.gateway.client;

import dev.kylebradshaw.gateway.dto.ProjectDto;
import dev.kylebradshaw.gateway.dto.ProjectStatsDto;
import dev.kylebradshaw.gateway.dto.TaskDto;
import dev.kylebradshaw.gateway.dto.VelocityDto;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;

@Component
public class TaskServiceClient {

    private final RestClient client;

    public TaskServiceClient(@Qualifier("taskRestClient") RestClient client) {
        this.client = client;
    }

    public List<ProjectDto> getMyProjects(String userId) {
        return client.get()
                .uri("/projects")
                .header("X-User-Id", userId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public ProjectDto getProject(String id) {
        return client.get()
                .uri("/projects/{id}", id)
                .retrieve()
                .body(ProjectDto.class);
    }

    public ProjectDto createProject(String userId, Map<String, Object> input) {
        return client.post()
                .uri("/projects")
                .header("X-User-Id", userId)
                .body(input)
                .retrieve()
                .body(ProjectDto.class);
    }

    public ProjectDto updateProject(String id, String userId, Map<String, Object> input) {
        return client.put()
                .uri("/projects/{id}", id)
                .header("X-User-Id", userId)
                .body(input)
                .retrieve()
                .body(ProjectDto.class);
    }

    public void deleteProject(String id, String userId) {
        client.delete()
                .uri("/projects/{id}", id)
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();
    }

    public TaskDto getTask(String id) {
        return client.get()
                .uri("/tasks/{id}", id)
                .retrieve()
                .body(TaskDto.class);
    }

    public List<TaskDto> getTasksByProject(String projectId) {
        return client.get()
                .uri("/tasks?projectId={projectId}", projectId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public TaskDto createTask(String userId, Map<String, Object> input) {
        return client.post()
                .uri("/tasks")
                .header("X-User-Id", userId)
                .body(input)
                .retrieve()
                .body(TaskDto.class);
    }

    public TaskDto updateTask(String id, String userId, Map<String, Object> input) {
        return client.put()
                .uri("/tasks/{id}", id)
                .header("X-User-Id", userId)
                .body(input)
                .retrieve()
                .body(TaskDto.class);
    }

    public TaskDto assignTask(String taskId, String assigneeId, String userId) {
        return client.put()
                .uri("/tasks/{taskId}/assign/{assigneeId}", taskId, assigneeId)
                .header("X-User-Id", userId)
                .retrieve()
                .body(TaskDto.class);
    }

    public void deleteTask(String id, String userId) {
        client.delete()
                .uri("/tasks/{id}", id)
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();
    }

    public ProjectStatsDto getProjectStats(String projectId) {
        return client.get()
                .uri("/analytics/projects/{id}/stats", projectId)
                .retrieve()
                .body(ProjectStatsDto.class);
    }

    public VelocityDto getProjectVelocity(String projectId, int weeks) {
        return client.get()
                .uri("/analytics/projects/{id}/velocity?weeks={weeks}", projectId, weeks)
                .retrieve()
                .body(VelocityDto.class);
    }
}
