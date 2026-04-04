package dev.kylebradshaw.gateway.dto;

import java.util.List;

public record NotificationDto(List<NotificationItem> notifications, int unreadCount) {
    public record NotificationItem(String id, String type, String message, String taskId, boolean read, String createdAt) {}
}
