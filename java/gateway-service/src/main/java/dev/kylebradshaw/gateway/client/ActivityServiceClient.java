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

    public List<ActivityEventDto> getActivityByTask(String taskId) {
        return client.get()
                .uri("/activity/task/{taskId}", taskId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public List<CommentDto> getCommentsByTask(String taskId) {
        return client.get()
                .uri("/comments/task/{taskId}", taskId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public ActivityStatsDto getActivityStats(String projectId, int weeks) {
        return client.get()
                .uri("/activity/project/{projectId}/stats?weeks={weeks}", projectId, weeks)
                .retrieve()
                .body(ActivityStatsDto.class);
    }

    public CommentDto addComment(String taskId, String userId, String body) {
        return client.post()
                .uri("/comments/task/{taskId}", taskId)
                .header("X-User-Id", userId)
                .body(Map.of("body", body))
                .retrieve()
                .body(CommentDto.class);
    }
}
