package dev.kylebradshaw.task.dto;

import java.io.Serializable;

public record PercentilesRow(double p50, double p75, double p95) implements Serializable {
}
