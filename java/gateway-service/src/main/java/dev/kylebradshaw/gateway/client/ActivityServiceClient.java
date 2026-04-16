package dev.kylebradshaw.gateway.client;

import dev.kylebradshaw.gateway.dto.ActivityEventDto;
import dev.kylebradshaw.gateway.dto.ActivityStatsDto;
import dev.kylebradshaw.gateway.dto.CommentDto;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;

@Component
public class ActivityServiceClient {

    private final RestClient client;

    public ActivityServiceClient(@Qualifier("activityRestClient") RestClient client) {
        this.client = client;
    }

    public List<ActivityEventDto> getActivityByTask(String taskId, String authHeader) {
        return client.get()
                .uri("/activity/task/{taskId}", taskId)
                .header("Authorization", authHeader)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public List<CommentDto> getCommentsByTask(String taskId, String authHeader) {
        return client.get()
                .uri("/comments/{taskId}", taskId)
                .header("Authorization", authHeader)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public ActivityStatsDto getActivityStats(String projectId, int weeks) {
        return client.get()
                .uri("/activity/project/{projectId}/stats?weeks={weeks}", projectId, weeks)
                .retrieve()
                .body(ActivityStatsDto.class);
    }

    public CommentDto addComment(String taskId, String authHeader, String body) {
        return client.post()
                .uri("/comments/{taskId}", taskId)
                .header("Authorization", authHeader)
                .body(Map.of("body", body))
                .retrieve()
                .body(CommentDto.class);
    }
}
