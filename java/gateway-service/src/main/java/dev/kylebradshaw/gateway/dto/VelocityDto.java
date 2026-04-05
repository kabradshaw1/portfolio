package dev.kylebradshaw.gateway.dto;

import java.util.List;

public record VelocityDto(
        List<WeeklyThroughputDto> weeklyThroughput,
        Double avgLeadTimeHours,
        PercentilesDto leadTimePercentiles) {

    public record WeeklyThroughputDto(String week, int completed, int created) {}

    public record PercentilesDto(double p50, double p75, double p95) {}
}
