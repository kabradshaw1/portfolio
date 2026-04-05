package dev.kylebradshaw.task.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.task.dto.MemberWorkloadRow;
import dev.kylebradshaw.task.dto.PercentilesRow;
import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.dto.WeeklyThroughputRow;
import dev.kylebradshaw.task.repository.AnalyticsRepository;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class AnalyticsServiceTest {

    @Mock private AnalyticsRepository analyticsRepo;

    private AnalyticsService service;

    @BeforeEach
    void setUp() {
        service = new AnalyticsService(analyticsRepo);
    }

    @Test
    void getProjectStats_assemblesAllMetrics() {
        UUID projectId = UUID.randomUUID();
        when(analyticsRepo.countByStatus(projectId)).thenReturn(Map.of("TODO", 3, "DONE", 5));
        when(analyticsRepo.countByPriority(projectId)).thenReturn(Map.of("HIGH", 2, "MEDIUM", 6));
        when(analyticsRepo.countOverdue(projectId)).thenReturn(1);
        when(analyticsRepo.avgCompletionTimeHours(projectId)).thenReturn(24.5);
        when(analyticsRepo.memberWorkload(projectId)).thenReturn(List.of(
                new MemberWorkloadRow(UUID.randomUUID(), "Alice", 3, 5)));

        ProjectStatsResponse result = service.getProjectStats(projectId);

        assertThat(result.taskCountByStatus()).containsEntry("TODO", 3);
        assertThat(result.taskCountByStatus()).containsEntry("DONE", 5);
        assertThat(result.overdueCount()).isEqualTo(1);
        assertThat(result.avgCompletionTimeHours()).isEqualTo(24.5);
        assertThat(result.memberWorkload()).hasSize(1);
    }

    @Test
    void getVelocityMetrics_assemblesAllMetrics() {
        UUID projectId = UUID.randomUUID();
        when(analyticsRepo.weeklyThroughput(projectId, 4)).thenReturn(List.of(
                new WeeklyThroughputRow("2026-W14", 5, 8)));
        when(analyticsRepo.avgCompletionTimeHours(projectId)).thenReturn(36.2);
        when(analyticsRepo.leadTimePercentiles(projectId)).thenReturn(
                new PercentilesRow(24.0, 48.0, 120.0));

        VelocityResponse result = service.getVelocityMetrics(projectId, 4);

        assertThat(result.weeklyThroughput()).hasSize(1);
        assertThat(result.weeklyThroughput().getFirst().week()).isEqualTo("2026-W14");
        assertThat(result.avgLeadTimeHours()).isEqualTo(36.2);
        assertThat(result.leadTimePercentiles().p50()).isEqualTo(24.0);
    }
}
