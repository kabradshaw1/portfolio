package dev.kylebradshaw.task.controller;

import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import dev.kylebradshaw.task.dto.MemberWorkloadRow;
import dev.kylebradshaw.task.dto.PercentilesRow;
import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.dto.WeeklyThroughputRow;
import dev.kylebradshaw.task.service.AnalyticsService;
import dev.kylebradshaw.task.service.ProjectService;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.test.context.support.WithMockUser;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

@WebMvcTest(AnalyticsController.class)
class AnalyticsControllerTest {

    @TestConfiguration
    static class TestSecurityConfig {
        @Bean
        public SecurityFilterChain testFilterChain(HttpSecurity http) throws Exception {
            return http.csrf(c -> c.disable())
                    .authorizeHttpRequests(a -> a.anyRequest().permitAll())
                    .build();
        }
    }

    @Autowired private MockMvc mockMvc;
    @MockitoBean private AnalyticsService analyticsService;
    @MockitoBean private ProjectService projectService;

    @Test
    @WithMockUser(username = "00000000-0000-0000-0000-000000000001")
    void getProjectStats_returns200() throws Exception {
        UUID projectId = UUID.randomUUID();
        var stats = new ProjectStatsResponse(
                Map.of("TODO", 3, "DONE", 5),
                Map.of("HIGH", 2),
                1, 24.5,
                List.of(new MemberWorkloadRow(UUID.randomUUID(), "Alice", 3, 5)));
        when(analyticsService.getProjectStats(projectId)).thenReturn(stats);

        mockMvc.perform(get("/analytics/projects/{id}/stats", projectId))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.overdueCount").value(1))
                .andExpect(jsonPath("$.avgCompletionTimeHours").value(24.5))
                .andExpect(jsonPath("$.memberWorkload[0].name").value("Alice"));
    }

    @Test
    @WithMockUser(username = "00000000-0000-0000-0000-000000000001")
    void getVelocityMetrics_returns200() throws Exception {
        UUID projectId = UUID.randomUUID();
        var velocity = new VelocityResponse(
                List.of(new WeeklyThroughputRow("2026-W14", 5, 8)),
                36.2,
                new PercentilesRow(24.0, 48.0, 120.0));
        when(analyticsService.getVelocityMetrics(projectId, 8)).thenReturn(velocity);

        mockMvc.perform(get("/analytics/projects/{id}/velocity", projectId))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.weeklyThroughput[0].week").value("2026-W14"))
                .andExpect(jsonPath("$.avgLeadTimeHours").value(36.2))
                .andExpect(jsonPath("$.leadTimePercentiles.p50").value(24.0));
    }

    @Test
    @WithMockUser(username = "00000000-0000-0000-0000-000000000001")
    void getVelocityMetrics_customWeeks() throws Exception {
        UUID projectId = UUID.randomUUID();
        var velocity = new VelocityResponse(List.of(), null, new PercentilesRow(0, 0, 0));
        when(analyticsService.getVelocityMetrics(projectId, 4)).thenReturn(velocity);

        mockMvc.perform(get("/analytics/projects/{id}/velocity", projectId)
                        .param("weeks", "4"))
                .andExpect(status().isOk());
    }
}
