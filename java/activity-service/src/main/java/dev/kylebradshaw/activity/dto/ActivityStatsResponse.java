package dev.kylebradshaw.activity.dto;

import java.util.List;

public record ActivityStatsResponse(
        int totalEvents,
        List<EventTypeCountRow> eventCountByType,
        int commentCount,
        int activeContributors,
        List<WeeklyActivityRow> weeklyActivity) {}
