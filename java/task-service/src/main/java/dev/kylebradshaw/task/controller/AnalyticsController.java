package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.service.AnalyticsService;
import dev.kylebradshaw.task.service.ProjectService;
import java.util.UUID;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/analytics")
public class AnalyticsController {

    private final AnalyticsService analyticsService;
    private final ProjectService projectService;

    public AnalyticsController(AnalyticsService analyticsService, ProjectService projectService) {
        this.analyticsService = analyticsService;
        this.projectService = projectService;
    }

    private UUID getAuthenticatedUserId() {
        var auth = SecurityContextHolder.getContext().getAuthentication();
        if (auth == null) {
            throw new IllegalStateException("No authenticated user");
        }
        return UUID.fromString(auth.getName());
    }

    @GetMapping("/projects/{id}/stats")
    public ProjectStatsResponse getProjectStats(@PathVariable UUID id) {
        UUID userId = getAuthenticatedUserId();
        projectService.getProject(id, userId);
        return analyticsService.getProjectStats(id);
    }

    @GetMapping("/projects/{id}/velocity")
    public VelocityResponse getVelocityMetrics(
            @PathVariable UUID id,
            @RequestParam(defaultValue = "8") int weeks) {
        UUID userId = getAuthenticatedUserId();
        projectService.getProject(id, userId);
        return analyticsService.getVelocityMetrics(id, weeks);
    }
}
