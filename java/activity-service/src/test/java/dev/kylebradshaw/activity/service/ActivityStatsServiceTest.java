package dev.kylebradshaw.activity.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.activity.dto.ActivityStatsResponse;
import dev.kylebradshaw.activity.dto.EventTypeCountRow;
import dev.kylebradshaw.activity.dto.WeeklyActivityRow;
import dev.kylebradshaw.activity.repository.ActivityStatsRepository;
import java.util.List;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class ActivityStatsServiceTest {

    @Mock private ActivityStatsRepository statsRepo;

    private ActivityStatsService service;

    @BeforeEach
    void setUp() {
        service = new ActivityStatsService(statsRepo);
    }

    @Test
    void getProjectStats_assemblesAllMetrics() {
        String projectId = "proj-123";
        when(statsRepo.countEvents(projectId)).thenReturn(142);
        when(statsRepo.countByEventType(projectId))
                .thenReturn(
                        List.of(
                                new EventTypeCountRow("task.created", 20),
                                new EventTypeCountRow("task.status_changed", 85)));
        when(statsRepo.countComments(projectId)).thenReturn(24);
        when(statsRepo.countActiveContributors(projectId)).thenReturn(5);
        when(statsRepo.weeklyActivity(projectId, 8))
                .thenReturn(List.of(new WeeklyActivityRow("2026-W14", 32, 6)));

        ActivityStatsResponse result = service.getProjectStats(projectId, 8);

        assertThat(result.totalEvents()).isEqualTo(142);
        assertThat(result.eventCountByType()).hasSize(2);
        assertThat(result.commentCount()).isEqualTo(24);
        assertThat(result.activeContributors()).isEqualTo(5);
        assertThat(result.weeklyActivity()).hasSize(1);
    }
}
