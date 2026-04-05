package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.entity.PasswordResetToken;
import dev.kylebradshaw.task.entity.RefreshToken;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.PasswordResetTokenRepository;
import dev.kylebradshaw.task.repository.RefreshTokenRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import dev.kylebradshaw.task.security.JwtService;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.UUID;

@Service
public class AuthService {

    private final UserRepository userRepository;
    private final RefreshTokenRepository refreshTokenRepository;
    private final JwtService jwtService;
    private final PasswordEncoder passwordEncoder;
    private final PasswordResetTokenRepository passwordResetTokenRepository;
    private final EmailService emailService;

    public AuthService(UserRepository userRepository,
                       RefreshTokenRepository refreshTokenRepository,
                       JwtService jwtService,
                       PasswordEncoder passwordEncoder,
                       PasswordResetTokenRepository passwordResetTokenRepository,
                       EmailService emailService) {
        this.userRepository = userRepository;
        this.refreshTokenRepository = refreshTokenRepository;
        this.jwtService = jwtService;
        this.passwordEncoder = passwordEncoder;
        this.passwordResetTokenRepository = passwordResetTokenRepository;
        this.emailService = emailService;
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
    public AuthResponse register(String email, String password, String name) {
        if (userRepository.findByEmail(email).isPresent()) {
            throw new IllegalArgumentException("Email already registered");
        }

        String hashedPassword = passwordEncoder.encode(password);
        User user = new User(email, name, hashedPassword, true);
        user = userRepository.save(user);

        return issueTokens(user);
    }

    @Transactional
    public AuthResponse login(String email, String password) {
        User user = userRepository.findByEmail(email)
                .orElseThrow(() -> new IllegalArgumentException("Invalid email or password"));

        if (user.getPasswordHash() == null || !passwordEncoder.matches(password, user.getPasswordHash())) {
            throw new IllegalArgumentException("Invalid email or password");
        }

        return issueTokens(user);
    }

    @Transactional
    public void forgotPassword(String email) {
        userRepository.findByEmail(email).ifPresent(user -> {
            String token = UUID.randomUUID().toString();
            Instant expiresAt = Instant.now().plusSeconds(3600);
            PasswordResetToken resetToken = new PasswordResetToken(token, user, expiresAt);
            passwordResetTokenRepository.save(resetToken);
            emailService.sendPasswordResetEmail(email, token);
        });
    }

    @Transactional
    public void resetPassword(String token, String newPassword) {
        PasswordResetToken resetToken = passwordResetTokenRepository.findByToken(token)
                .orElseThrow(() -> new IllegalArgumentException("Invalid reset token"));

        if (resetToken.isExpired()) {
            throw new IllegalArgumentException("Reset token has expired");
        }

        User user = resetToken.getUser();
        user.setPasswordHash(passwordEncoder.encode(newPassword));
        userRepository.save(user);
        passwordResetTokenRepository.delete(resetToken);
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
