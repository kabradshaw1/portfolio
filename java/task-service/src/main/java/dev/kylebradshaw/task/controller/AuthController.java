package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.AuthRequest;
import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.dto.ForgotPasswordRequest;
import dev.kylebradshaw.task.dto.LoginRequest;
import dev.kylebradshaw.task.dto.RegisterRequest;
import dev.kylebradshaw.task.dto.ResetPasswordRequest;
import dev.kylebradshaw.task.service.AuthService;
import jakarta.validation.Valid;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.util.LinkedMultiValueMap;
import org.springframework.util.MultiValueMap;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.client.RestClient;

import java.util.Map;

@RestController
@RequestMapping("/api/auth")
public class AuthController {

    private final AuthService authService;
    private final RestClient restClient;

    @Value("${app.google.client-id}")
    private String googleClientId;

    @Value("${app.google.client-secret}")
    private String googleClientSecret;

    @Value("${app.google.token-url:https://oauth2.googleapis.com/token}")
    private String googleTokenUrl;

    @Value("${app.google.userinfo-url:https://www.googleapis.com/oauth2/v3/userinfo}")
    private String googleUserInfoUrl;

    public AuthController(AuthService authService) {
        this.authService = authService;
        this.restClient = RestClient.create();
    }

    /**
     * Exchange Google authorization code for user info and issue tokens.
     * POST /api/auth/google
     */
    @PostMapping("/google")
    public AuthResponse googleLogin(@Valid @RequestBody AuthRequest request) {
        // Exchange authorization code for tokens at Google's token endpoint
        MultiValueMap<String, String> tokenParams = new LinkedMultiValueMap<>();
        tokenParams.add("code", request.code());
        tokenParams.add("client_id", googleClientId);
        tokenParams.add("client_secret", googleClientSecret);
        tokenParams.add("redirect_uri", request.redirectUri());
        tokenParams.add("grant_type", "authorization_code");

        @SuppressWarnings("unchecked")
        Map<String, Object> tokenResponse = restClient.post()
                .uri(googleTokenUrl)
                .contentType(MediaType.APPLICATION_FORM_URLENCODED)
                .body(tokenParams)
                .retrieve()
                .body(Map.class);

        String accessTokenGoogle = (String) tokenResponse.get("access_token");

        // Fetch user info using the Google access token
        @SuppressWarnings("unchecked")
        Map<String, Object> userInfo = restClient.get()
                .uri(googleUserInfoUrl)
                .header("Authorization", "Bearer " + accessTokenGoogle)
                .retrieve()
                .body(Map.class);

        String email = (String) userInfo.get("email");
        String name = (String) userInfo.get("name");
        String avatarUrl = (String) userInfo.get("picture");

        return authService.authenticateGoogleUser(email, name, avatarUrl);
    }

    /**
     * Refresh access token using a valid refresh token.
     * POST /api/auth/refresh
     */
    @PostMapping("/refresh")
    public AuthResponse refresh(@RequestBody Map<String, String> body) {
        String refreshToken = body.get("refreshToken");
        return authService.refreshAccessToken(refreshToken);
    }

    @PostMapping("/register")
    public AuthResponse register(@Valid @RequestBody RegisterRequest request) {
        return authService.register(request.email(), request.password(), request.name());
    }

    @PostMapping("/login")
    public AuthResponse login(@Valid @RequestBody LoginRequest request) {
        return authService.login(request.email(), request.password());
    }

    @PostMapping("/forgot-password")
    public ResponseEntity<Void> forgotPassword(@Valid @RequestBody ForgotPasswordRequest request) {
        authService.forgotPassword(request.email());
        return ResponseEntity.noContent().build();
    }

    @PostMapping("/reset-password")
    public ResponseEntity<Void> resetPassword(@Valid @RequestBody ResetPasswordRequest request) {
        authService.resetPassword(request.token(), request.password());
        return ResponseEntity.noContent().build();
    }
}
