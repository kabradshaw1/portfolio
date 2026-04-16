package dev.kylebradshaw.task.dto;

import jakarta.validation.constraints.NotBlank;

public record AuthRequest(
        @NotBlank String code,
        @NotBlank String redirectUri,
        String state
) {}
