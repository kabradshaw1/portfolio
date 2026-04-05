package dev.kylebradshaw.gateway.resolver;

import dev.kylebradshaw.gateway.client.ActivityServiceClient;
import dev.kylebradshaw.gateway.client.TaskServiceClient;
import dev.kylebradshaw.gateway.dto.ActivityStatsDto;
import dev.kylebradshaw.gateway.dto.ProjectHealthDto;
import dev.kylebradshaw.gateway.dto.ProjectStatsDto;
import dev.kylebradshaw.gateway.dto.VelocityDto;
import org.springframework.cache.annotation.Cacheable;
import org.springframework.graphql.data.method.annotation.Argument;
import org.springframework.graphql.data.method.annotation.QueryMapping;
import org.springframework.stereotype.Controller;

@Controller
public class AnalyticsResolver {

    private final TaskServiceClient taskClient;
    private final ActivityServiceClient activityClient;

    public AnalyticsResolver(TaskServiceClient taskClient, ActivityServiceClient activityClient) {
        this.taskClient = taskClient;
        this.activityClient = activityClient;
    }

    @QueryMapping
    public ProjectStatsDto projectStats(@Argument String projectId) {
        return taskClient.getProjectStats(projectId);
    }

    @QueryMapping
    public VelocityDto projectVelocity(@Argument String projectId, @Argument Integer weeks) {
        int w = weeks != null ? weeks : 8;
        return taskClient.getProjectVelocity(projectId, w);
    }

    @QueryMapping
    @Cacheable(value = "project-health", key = "#projectId")
    public ProjectHealthDto projectHealth(@Argument String projectId) {
        ProjectStatsDto stats = taskClient.getProjectStats(projectId);
        VelocityDto velocity = taskClient.getProjectVelocity(projectId, 8);
        ActivityStatsDto activity = activityClient.getActivityStats(projectId, 8);
        return new ProjectHealthDto(stats, velocity, activity);
    }
}
