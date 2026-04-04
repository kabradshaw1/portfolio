package dev.kylebradshaw.gateway.client;

import dev.kylebradshaw.gateway.dto.NotificationDto;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

@Component
public class NotificationServiceClient {

    private final RestClient client;

    public NotificationServiceClient(@Qualifier("notificationRestClient") RestClient client) {
        this.client = client;
    }

    public NotificationDto getNotifications(String userId, Boolean unreadOnly) {
        String uri = unreadOnly != null && unreadOnly
                ? "/notifications?unreadOnly=true"
                : "/notifications";
        return client.get()
                .uri(uri)
                .header("X-User-Id", userId)
                .retrieve()
                .body(NotificationDto.class);
    }

    public void markRead(String userId, String notificationId) {
        client.put()
                .uri("/notifications/{id}/read", notificationId)
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();
    }

    public void markAllRead(String userId) {
        client.put()
                .uri("/notifications/read-all")
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();
    }
}
