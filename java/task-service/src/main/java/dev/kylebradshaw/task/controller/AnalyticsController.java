package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.service.AnalyticsService;
import java.util.UUID;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/analytics")
public class AnalyticsController {

    private final AnalyticsService analyticsService;

    public AnalyticsController(AnalyticsService analyticsService) {
        this.analyticsService = analyticsService;
    }

    @GetMapping("/projects/{id}/stats")
    public ProjectStatsResponse getProjectStats(@PathVariable UUID id) {
        return analyticsService.getProjectStats(id);
    }

    @GetMapping("/projects/{id}/velocity")
    public VelocityResponse getVelocityMetrics(
            @PathVariable UUID id,
            @RequestParam(defaultValue = "8") int weeks) {
        return analyticsService.getVelocityMetrics(id, weeks);
    }
}
