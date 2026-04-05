package dev.kylebradshaw.activity.service;

import dev.kylebradshaw.activity.dto.ActivityStatsResponse;
import dev.kylebradshaw.activity.repository.ActivityStatsRepository;
import org.springframework.stereotype.Service;

@Service
public class ActivityStatsService {

    private final ActivityStatsRepository statsRepo;

    public ActivityStatsService(ActivityStatsRepository statsRepo) {
        this.statsRepo = statsRepo;
    }

    public ActivityStatsResponse getProjectStats(String projectId, int weeks) {
        return new ActivityStatsResponse(
                statsRepo.countEvents(projectId),
                statsRepo.countByEventType(projectId),
                statsRepo.countComments(projectId),
                statsRepo.countActiveContributors(projectId),
                statsRepo.weeklyActivity(projectId, weeks));
    }
}
