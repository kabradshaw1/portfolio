package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.repository.AnalyticsRepository;
import java.util.UUID;
import org.springframework.stereotype.Service;

@Service
public class AnalyticsService {

    private final AnalyticsRepository analyticsRepo;

    public AnalyticsService(AnalyticsRepository analyticsRepo) {
        this.analyticsRepo = analyticsRepo;
    }

    public ProjectStatsResponse getProjectStats(UUID projectId) {
        return new ProjectStatsResponse(
                analyticsRepo.countByStatus(projectId),
                analyticsRepo.countByPriority(projectId),
                analyticsRepo.countOverdue(projectId),
                analyticsRepo.avgCompletionTimeHours(projectId),
                analyticsRepo.memberWorkload(projectId));
    }

    public VelocityResponse getVelocityMetrics(UUID projectId, int weeks) {
        return new VelocityResponse(
                analyticsRepo.weeklyThroughput(projectId, weeks),
                analyticsRepo.avgCompletionTimeHours(projectId),
                analyticsRepo.leadTimePercentiles(projectId));
    }
}
