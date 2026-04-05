package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.dto.ProjectResponse;
import dev.kylebradshaw.task.service.ProjectService;
import jakarta.validation.Valid;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/projects")
public class ProjectController {

    private final ProjectService projectService;

    public ProjectController(ProjectService projectService) {
        this.projectService = projectService;
    }

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public ProjectResponse createProject(
            @Valid @RequestBody CreateProjectRequest request,
            @RequestHeader("X-User-Id") UUID userId) {
        return ProjectResponse.from(projectService.createProject(request, userId));
    }

    @GetMapping
    public List<ProjectResponse> getMyProjects(@RequestHeader("X-User-Id") UUID userId) {
        return projectService.getProjectsForUser(userId).stream()
                .map(ProjectResponse::from)
                .toList();
    }

    @GetMapping("/{id}")
    public ProjectResponse getProject(@PathVariable UUID id) {
        return ProjectResponse.from(projectService.getProject(id));
    }

    @PutMapping("/{id}")
    public ProjectResponse updateProject(
            @PathVariable UUID id,
            @RequestHeader("X-User-Id") UUID userId,
            @RequestBody Map<String, String> body) {
        return ProjectResponse.from(
                projectService.updateProject(id, userId, body.get("name"), body.get("description")));
    }

    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void deleteProject(
            @PathVariable UUID id,
            @RequestHeader("X-User-Id") UUID userId) {
        projectService.deleteProject(id, userId);
    }
}
