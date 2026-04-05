package dev.kylebradshaw.gateway.dto;

public record ProjectHealthDto(
        ProjectStatsDto stats,
        VelocityDto velocity,
        ActivityStatsDto activity) {}
