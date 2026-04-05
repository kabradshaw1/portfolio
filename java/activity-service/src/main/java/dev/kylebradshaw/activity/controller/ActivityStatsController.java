package dev.kylebradshaw.activity.controller;

import dev.kylebradshaw.activity.dto.ActivityStatsResponse;
import dev.kylebradshaw.activity.service.ActivityStatsService;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/activity")
public class ActivityStatsController {

    private final ActivityStatsService statsService;

    public ActivityStatsController(ActivityStatsService statsService) {
        this.statsService = statsService;
    }

    @GetMapping("/project/{projectId}/stats")
    public ActivityStatsResponse getProjectStats(
            @PathVariable String projectId,
            @RequestParam(defaultValue = "8") int weeks) {
        return statsService.getProjectStats(projectId, weeks);
    }
}
