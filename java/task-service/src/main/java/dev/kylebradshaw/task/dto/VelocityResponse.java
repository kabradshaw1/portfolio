package dev.kylebradshaw.task.dto;

import java.io.Serializable;
import java.util.List;

public record VelocityResponse(
        List<WeeklyThroughputRow> weeklyThroughput,
        Double avgLeadTimeHours,
        PercentilesRow leadTimePercentiles) implements Serializable {
}
