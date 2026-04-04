package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.entity.RefreshToken;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.RefreshTokenRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import dev.kylebradshaw.task.security.JwtService;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;

@Service
public class AuthService {

    private final UserRepository userRepository;
    private final RefreshTokenRepository refreshTokenRepository;
    private final JwtService jwtService;

    public AuthService(UserRepository userRepository,
                       RefreshTokenRepository refreshTokenRepository,
                       JwtService jwtService) {
        this.userRepository = userRepository;
        this.refreshTokenRepository = refreshTokenRepository;
        this.jwtService = jwtService;
    }

    @Transactional
    public AuthResponse authenticateGoogleUser(String email, String name, String avatarUrl) {
        User user = userRepository.findByEmail(email).orElse(null);

        if (user == null) {
            user = new User(email, name, avatarUrl);
            user = userRepository.save(user);
        } else {
            user.setName(name);
            user.setAvatarUrl(avatarUrl);
        }

        return issueTokens(user);
    }

    @Transactional
    public AuthResponse refreshAccessToken(String refreshTokenStr) {
        RefreshToken refreshToken = refreshTokenRepository.findByToken(refreshTokenStr)
                .orElseThrow(() -> new IllegalArgumentException("Refresh token not found"));

        if (refreshToken.isExpired()) {
            throw new IllegalArgumentException("Refresh token is expired");
        }

        User user = refreshToken.getUser();
        return issueTokens(user);
    }

    private AuthResponse issueTokens(User user) {
        String accessToken = jwtService.generateAccessToken(user.getId(), user.getEmail());
        String refreshTokenStr = jwtService.generateRefreshTokenString();
        Instant expiresAt = Instant.now().plusMillis(jwtService.getRefreshTokenTtlMs());

        RefreshToken refreshToken = new RefreshToken(user, refreshTokenStr, expiresAt);
        refreshTokenRepository.save(refreshToken);

        return new AuthResponse(
                accessToken,
                refreshTokenStr,
                user.getId(),
                user.getEmail(),
                user.getName(),
                user.getAvatarUrl()
        );
    }
}
