package dev.kylebradshaw.task.dto;

import java.io.Serializable;

public record WeeklyThroughputRow(String week, int completed, int created) implements Serializable {
}
