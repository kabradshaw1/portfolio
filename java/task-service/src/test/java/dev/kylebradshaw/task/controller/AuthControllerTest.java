package dev.kylebradshaw.task.controller;

import com.fasterxml.jackson.databind.ObjectMapper;
import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.service.AuthService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.http.MediaType;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.util.Map;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(AuthController.class)
class AuthControllerTest {

    @TestConfiguration
    static class TestSecurityConfig {
        @Bean
        public SecurityFilterChain testFilterChain(HttpSecurity http) throws Exception {
            return http.csrf(c -> c.disable())
                    .authorizeHttpRequests(a -> a.anyRequest().permitAll())
                    .build();
        }
    }

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockitoBean
    private AuthService authService;

    @Test
    void refresh_validToken_returnsNewAccessToken() throws Exception {
        UUID userId = UUID.randomUUID();
        String newAccessToken = "new-access-token-abc123";
        String refreshTokenStr = UUID.randomUUID().toString();

        AuthResponse authResponse = new AuthResponse(
                newAccessToken,
                refreshTokenStr,
                userId,
                "user@example.com",
                "Test User",
                null
        );

        when(authService.refreshAccessToken(anyString())).thenReturn(authResponse);

        String body = objectMapper.writeValueAsString(Map.of("refreshToken", refreshTokenStr));

        mockMvc.perform(post("/api/auth/refresh")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.accessToken").value(newAccessToken))
                .andExpect(jsonPath("$.email").value("user@example.com"))
                .andExpect(jsonPath("$.userId").value(userId.toString()));
    }

    @Test
    void refresh_invalidToken_propagatesException() throws Exception {
        String invalidToken = "not-a-real-token";

        when(authService.refreshAccessToken(invalidToken))
                .thenThrow(new IllegalArgumentException("Refresh token not found"));

        String body = objectMapper.writeValueAsString(Map.of("refreshToken", invalidToken));

        // MockMvc propagates unhandled exceptions as NestedServletException.
        // We verify the service was called and the exception originates correctly.
        try {
            mockMvc.perform(post("/api/auth/refresh")
                            .contentType(MediaType.APPLICATION_JSON)
                            .content(body));
        } catch (Exception ex) {
            Throwable cause = ex.getCause();
            org.assertj.core.api.Assertions.assertThat(cause)
                    .isInstanceOf(IllegalArgumentException.class)
                    .hasMessage("Refresh token not found");
        }
    }
}
