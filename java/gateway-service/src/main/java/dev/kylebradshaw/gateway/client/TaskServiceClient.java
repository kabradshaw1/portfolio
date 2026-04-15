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

    public List<ProjectDto> getMyProjects(String authHeader) {
        return client.get()
                .uri("/projects")
                .header("Authorization", authHeader)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public ProjectDto getProject(String id, String authHeader) {
        return client.get()
                .uri("/projects/{id}", id)
                .header("Authorization", authHeader)
                .retrieve()
                .body(ProjectDto.class);
    }

    public ProjectDto createProject(String authHeader, Map<String, Object> input) {
        return client.post()
                .uri("/projects")
                .header("Authorization", authHeader)
                .body(input)
                .retrieve()
                .body(ProjectDto.class);
    }

    public ProjectDto updateProject(String id, String authHeader, Map<String, Object> input) {
        return client.put()
                .uri("/projects/{id}", id)
                .header("Authorization", authHeader)
                .body(input)
                .retrieve()
                .body(ProjectDto.class);
    }

    public void deleteProject(String id, String authHeader) {
        client.delete()
                .uri("/projects/{id}", id)
                .header("Authorization", authHeader)
                .retrieve()
                .toBodilessEntity();
    }

    public TaskDto getTask(String id, String authHeader) {
        return client.get()
                .uri("/tasks/{id}", id)
                .header("Authorization", authHeader)
                .retrieve()
                .body(TaskDto.class);
    }

    public List<TaskDto> getTasksByProject(String projectId, String authHeader) {
        return client.get()
                .uri("/tasks?projectId={projectId}", projectId)
                .header("Authorization", authHeader)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public TaskDto createTask(String authHeader, Map<String, Object> input) {
        return client.post()
                .uri("/tasks")
                .header("Authorization", authHeader)
                .body(input)
                .retrieve()
                .body(TaskDto.class);
    }

    public TaskDto updateTask(String id, String authHeader, Map<String, Object> input) {
        return client.put()
                .uri("/tasks/{id}", id)
                .header("Authorization", authHeader)
                .body(input)
                .retrieve()
                .body(TaskDto.class);
    }

    public TaskDto assignTask(String taskId, String assigneeId, String authHeader) {
        return client.put()
                .uri("/tasks/{taskId}/assign/{assigneeId}", taskId, assigneeId)
                .header("Authorization", authHeader)
                .retrieve()
                .body(TaskDto.class);
    }

    public void deleteTask(String id, String authHeader) {
        client.delete()
                .uri("/tasks/{id}", id)
                .header("Authorization", authHeader)
                .retrieve()
                .toBodilessEntity();
    }

    public ProjectStatsDto getProjectStats(String projectId) {
        ProjectStatsDto.Raw raw = client.get()
                .uri("/analytics/projects/{id}/stats", projectId)
                .retrieve()
                .body(ProjectStatsDto.Raw.class);
        return raw == null ? null : raw.toTyped();
    }

    public VelocityDto getProjectVelocity(String projectId, int weeks) {
        return client.get()
                .uri("/analytics/projects/{id}/velocity?weeks={weeks}", projectId, weeks)
                .retrieve()
                .body(VelocityDto.class);
    }

    public void deleteUser(String authHeader) {
        client.delete()
                .uri("/auth/user")
                .header("Authorization", authHeader)
                .retrieve()
                .toBodilessEntity();
    }
}
