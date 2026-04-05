package dev.kylebradshaw.gateway.dto;

import java.util.List;

public record ActivityStatsDto(
        int totalEvents,
        List<EventTypeCountDto> eventCountByType,
        int commentCount,
        int activeContributors,
        List<WeeklyActivityDto> weeklyActivity) {

    public record EventTypeCountDto(String eventType, int count) {}

    public record WeeklyActivityDto(String week, int events, int comments) {}
}
