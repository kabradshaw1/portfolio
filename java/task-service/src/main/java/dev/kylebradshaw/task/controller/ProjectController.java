package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.dto.ProjectResponse;
import dev.kylebradshaw.task.service.ProjectService;
import jakarta.validation.Valid;
import java.util.List;
import java.util.Map;
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
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/projects")
public class ProjectController {

    private final ProjectService projectService;

    public ProjectController(ProjectService projectService) {
        this.projectService = projectService;
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
    public ProjectResponse createProject(@Valid @RequestBody CreateProjectRequest request) {
        UUID userId = getAuthenticatedUserId();
        return ProjectResponse.from(projectService.createProject(request, userId));
    }

    @GetMapping
    public List<ProjectResponse> getMyProjects() {
        UUID userId = getAuthenticatedUserId();
        return projectService.getProjectsForUser(userId).stream()
                .map(ProjectResponse::from)
                .toList();
    }

    @GetMapping("/{id}")
    public ProjectResponse getProject(@PathVariable UUID id) {
        UUID userId = getAuthenticatedUserId();
        return ProjectResponse.from(projectService.getProject(id, userId));
    }

    @PutMapping("/{id}")
    public ProjectResponse updateProject(
            @PathVariable UUID id,
            @RequestBody Map<String, String> body) {
        UUID userId = getAuthenticatedUserId();
        return ProjectResponse.from(
                projectService.updateProject(id, userId, body.get("name"), body.get("description")));
    }

    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void deleteProject(@PathVariable UUID id) {
        UUID userId = getAuthenticatedUserId();
        projectService.deleteProject(id, userId);
    }
}
