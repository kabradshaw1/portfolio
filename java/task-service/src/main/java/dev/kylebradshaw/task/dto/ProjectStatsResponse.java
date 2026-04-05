package dev.kylebradshaw.task.dto;

import java.io.Serializable;
import java.util.List;
import java.util.Map;

public record ProjectStatsResponse(
        Map<String, Integer> taskCountByStatus,
        Map<String, Integer> taskCountByPriority,
        int overdueCount,
        Double avgCompletionTimeHours,
        List<MemberWorkloadRow> memberWorkload) implements Serializable {
}
