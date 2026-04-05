package dev.kylebradshaw.gateway.dto;

import java.util.List;
import java.util.Map;

public record ProjectStatsDto(
        Map<String, Integer> taskCountByStatus,
        Map<String, Integer> taskCountByPriority,
        int overdueCount,
        Double avgCompletionTimeHours,
        List<MemberWorkloadDto> memberWorkload) {

    public record MemberWorkloadDto(String userId, String name, int assignedCount, int completedCount) {}
}
