package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.AuthRequest;
import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.dto.ForgotPasswordRequest;
import dev.kylebradshaw.task.dto.LoginRequest;
import dev.kylebradshaw.task.dto.RegisterRequest;
import dev.kylebradshaw.task.dto.ResetPasswordRequest;
import dev.kylebradshaw.task.service.AuthService;
import jakarta.servlet.http.Cookie;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.validation.Valid;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.util.LinkedMultiValueMap;
import org.springframework.util.MultiValueMap;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.client.RestClient;

import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/auth")
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

    private String resolveSameSite() {
        String value = System.getenv().getOrDefault("COOKIE_SAMESITE", "lax");
        return switch (value.toLowerCase()) {
            case "none" -> "None";
            case "strict" -> "Strict";
            default -> "Lax";
        };
    }

    private void setAuthCookies(HttpServletResponse response, String accessToken, String refreshToken) {
        boolean secure = Boolean.parseBoolean(
                System.getenv().getOrDefault("COOKIE_SECURE", "false"));
        String domain = System.getenv().getOrDefault("COOKIE_DOMAIN", "");
        String sameSite = resolveSameSite();

        Cookie accessCookie = new Cookie("access_token", accessToken);
        accessCookie.setHttpOnly(true);
        accessCookie.setSecure(secure);
        accessCookie.setPath("/");
        accessCookie.setMaxAge(900); // 15 min
        accessCookie.setAttribute("SameSite", sameSite);
        if (!domain.isEmpty()) {
            accessCookie.setDomain(domain);
        }
        response.addCookie(accessCookie);

        Cookie refreshCookie = new Cookie("refresh_token", refreshToken);
        refreshCookie.setHttpOnly(true);
        refreshCookie.setSecure(secure);
        refreshCookie.setPath("/auth");
        refreshCookie.setMaxAge(604800); // 7 days
        refreshCookie.setAttribute("SameSite", sameSite);
        if (!domain.isEmpty()) {
            refreshCookie.setDomain(domain);
        }
        response.addCookie(refreshCookie);
    }

    private void clearAuthCookies(HttpServletResponse response) {
        boolean secure = Boolean.parseBoolean(
                System.getenv().getOrDefault("COOKIE_SECURE", "false"));
        String domain = System.getenv().getOrDefault("COOKIE_DOMAIN", "");
        String sameSite = resolveSameSite();

        Cookie accessCookie = new Cookie("access_token", "");
        accessCookie.setHttpOnly(true);
        accessCookie.setSecure(secure);
        accessCookie.setPath("/");
        accessCookie.setMaxAge(0);
        accessCookie.setAttribute("SameSite", sameSite);
        if (!domain.isEmpty()) {
            accessCookie.setDomain(domain);
        }
        response.addCookie(accessCookie);

        Cookie refreshCookie = new Cookie("refresh_token", "");
        refreshCookie.setHttpOnly(true);
        refreshCookie.setSecure(secure);
        refreshCookie.setPath("/auth");
        refreshCookie.setMaxAge(0);
        refreshCookie.setAttribute("SameSite", sameSite);
        if (!domain.isEmpty()) {
            refreshCookie.setDomain(domain);
        }
        response.addCookie(refreshCookie);
    }

    /**
     * Exchange Google authorization code for user info and issue tokens.
     * POST /auth/google
     */
    @PostMapping("/google")
    public Map<String, Object> googleLogin(
            @Valid @RequestBody AuthRequest request,
            HttpServletResponse response) {
        if (request.state() == null || request.state().isBlank()) {
            throw new IllegalArgumentException("OAuth state parameter required");
        }
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

        AuthResponse authResponse = authService.authenticateGoogleUser(email, name, avatarUrl);
        setAuthCookies(response, authResponse.accessToken(), authResponse.refreshToken());

        return Map.of(
                "userId", authResponse.userId(),
                "email", authResponse.email(),
                "name", authResponse.name(),
                "avatarUrl", authResponse.avatarUrl() != null ? authResponse.avatarUrl() : ""
        );
    }

    /**
     * Refresh access token using a valid refresh token (from cookie or JSON body).
     * POST /auth/refresh
     */
    @PostMapping("/refresh")
    public Map<String, Object> refresh(HttpServletRequest request, HttpServletResponse response) {
        String refreshToken = null;
        if (request.getCookies() != null) {
            for (Cookie cookie : request.getCookies()) {
                if ("refresh_token".equals(cookie.getName())) {
                    refreshToken = cookie.getValue();
                    break;
                }
            }
        }
        if (refreshToken == null || refreshToken.isBlank()) {
            throw new IllegalArgumentException("Refresh token not found");
        }

        AuthResponse authResponse = authService.refreshAccessToken(refreshToken);
        setAuthCookies(response, authResponse.accessToken(), authResponse.refreshToken());

        return Map.of(
                "userId", authResponse.userId(),
                "email", authResponse.email(),
                "name", authResponse.name(),
                "avatarUrl", authResponse.avatarUrl() != null ? authResponse.avatarUrl() : ""
        );
    }

    @PostMapping("/register")
    public Map<String, Object> register(
            @Valid @RequestBody RegisterRequest request,
            HttpServletResponse response) {
        AuthResponse authResponse = authService.register(request.email(), request.password(), request.name());
        setAuthCookies(response, authResponse.accessToken(), authResponse.refreshToken());

        return Map.of(
                "userId", authResponse.userId(),
                "email", authResponse.email(),
                "name", authResponse.name(),
                "avatarUrl", authResponse.avatarUrl() != null ? authResponse.avatarUrl() : ""
        );
    }

    @PostMapping("/login")
    public Map<String, Object> login(
            @Valid @RequestBody LoginRequest request,
            HttpServletResponse response) {
        AuthResponse authResponse = authService.login(request.email(), request.password());
        setAuthCookies(response, authResponse.accessToken(), authResponse.refreshToken());

        return Map.of(
                "userId", authResponse.userId(),
                "email", authResponse.email(),
                "name", authResponse.name(),
                "avatarUrl", authResponse.avatarUrl() != null ? authResponse.avatarUrl() : ""
        );
    }

    @PostMapping("/logout")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void logout(HttpServletResponse response) {
        clearAuthCookies(response);
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

    @DeleteMapping("/user")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void deleteUser(@RequestHeader("X-User-Id") UUID userId) {
        authService.deleteUser(userId);
    }
}
