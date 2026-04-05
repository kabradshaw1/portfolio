package dev.kylebradshaw.notification.controller;

import dev.kylebradshaw.notification.dto.NotificationResponse;
import dev.kylebradshaw.notification.service.NotificationService;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/notifications")
public class NotificationController {

    private final NotificationService notificationService;

    public NotificationController(NotificationService notificationService) {
        this.notificationService = notificationService;
    }

    @GetMapping
    public ResponseEntity<NotificationResponse> getNotifications(
            @RequestHeader("X-User-Id") String userId,
            @RequestParam(defaultValue = "false") boolean unreadOnly) {
        return ResponseEntity.ok(notificationService.getNotifications(userId, unreadOnly));
    }

    @PostMapping("/{id}/read")
    public ResponseEntity<Void> markRead(
            @RequestHeader("X-User-Id") String userId,
            @PathVariable String id) {
        notificationService.markRead(userId, id);
        return ResponseEntity.noContent().build();
    }

    @PostMapping("/read-all")
    public ResponseEntity<Void> markAllRead(
            @RequestHeader("X-User-Id") String userId) {
        notificationService.markAllRead(userId);
        return ResponseEntity.noContent().build();
    }
}
