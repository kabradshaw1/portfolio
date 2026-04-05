package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.entity.PasswordResetToken;
import dev.kylebradshaw.task.entity.RefreshToken;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.PasswordResetTokenRepository;
import dev.kylebradshaw.task.repository.RefreshTokenRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import dev.kylebradshaw.task.security.JwtService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;

import java.time.Instant;
import java.util.Optional;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.argThat;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AuthServiceTest {

    @Mock
    private UserRepository userRepository;

    @Mock
    private RefreshTokenRepository refreshTokenRepository;

    @Mock
    private JwtService jwtService;

    @Mock
    private PasswordResetTokenRepository passwordResetTokenRepository;

    @Mock
    private EmailService emailService;

    private final PasswordEncoder passwordEncoder = new BCryptPasswordEncoder();

    private AuthService authService;

    @BeforeEach
    void setUp() {
        authService = new AuthService(userRepository, refreshTokenRepository,
                jwtService, passwordEncoder, passwordResetTokenRepository, emailService);
    }

    @Test
    void authenticateGoogleUser_existingUser_returnsTokens() {
        String email = "existing@example.com";
        String name = "Existing User";
        String avatarUrl = "https://example.com/avatar.png";

        User existingUser = new User(email, name, avatarUrl);
        String accessToken = "access-token-123";
        String refreshTokenStr = UUID.randomUUID().toString();

        when(userRepository.findByEmail(email)).thenReturn(Optional.of(existingUser));
        when(jwtService.generateAccessToken(any(), eq(email))).thenReturn(accessToken);
        when(jwtService.generateRefreshTokenString()).thenReturn(refreshTokenStr);
        when(jwtService.getRefreshTokenTtlMs()).thenReturn(604_800_000L);
        when(refreshTokenRepository.save(any())).thenAnswer(inv -> inv.getArgument(0));

        AuthResponse response = authService.authenticateGoogleUser(email, name, avatarUrl);

        assertThat(response.accessToken()).isEqualTo(accessToken);
        assertThat(response.refreshToken()).isEqualTo(refreshTokenStr);
        assertThat(response.email()).isEqualTo(email);
        assertThat(response.name()).isEqualTo(name);
        // Existing user should NOT be created again
        verify(userRepository, never()).save(argThat(u -> u.getEmail().equals(email) && u.getId() == null));
    }

    @Test
    void authenticateGoogleUser_newUser_createsUserAndReturnsTokens() {
        String email = "new@example.com";
        String name = "New User";
        String avatarUrl = "https://example.com/new-avatar.png";

        String accessToken = "new-access-token";
        String refreshTokenStr = UUID.randomUUID().toString();

        when(userRepository.findByEmail(email)).thenReturn(Optional.empty());
        // Return saved user (simulate DB assigning ID)
        when(userRepository.save(any(User.class))).thenAnswer(inv -> inv.getArgument(0));
        when(jwtService.generateAccessToken(any(), eq(email))).thenReturn(accessToken);
        when(jwtService.generateRefreshTokenString()).thenReturn(refreshTokenStr);
        when(jwtService.getRefreshTokenTtlMs()).thenReturn(604_800_000L);
        when(refreshTokenRepository.save(any())).thenAnswer(inv -> inv.getArgument(0));

        AuthResponse response = authService.authenticateGoogleUser(email, name, avatarUrl);

        assertThat(response.accessToken()).isEqualTo(accessToken);
        assertThat(response.refreshToken()).isEqualTo(refreshTokenStr);
        assertThat(response.email()).isEqualTo(email);
        assertThat(response.name()).isEqualTo(name);
        // New user should be saved
        verify(userRepository).save(any(User.class));
    }

    @Test
    void refreshAccessToken_validToken_returnsNewAccessToken() {
        String refreshTokenStr = UUID.randomUUID().toString();
        String email = "user@example.com";
        String newAccessToken = "new-access-token-456";

        User user = new User(email, "User Name", null);
        RefreshToken refreshToken = new RefreshToken(
                user, refreshTokenStr, Instant.now().plusSeconds(3600));

        when(refreshTokenRepository.findByToken(refreshTokenStr)).thenReturn(Optional.of(refreshToken));
        when(jwtService.generateAccessToken(any(), eq(email))).thenReturn(newAccessToken);
        when(jwtService.generateRefreshTokenString()).thenReturn(refreshTokenStr);
        when(jwtService.getRefreshTokenTtlMs()).thenReturn(604_800_000L);
        when(refreshTokenRepository.save(any())).thenAnswer(inv -> inv.getArgument(0));

        AuthResponse response = authService.refreshAccessToken(refreshTokenStr);

        assertThat(response.accessToken()).isEqualTo(newAccessToken);
        assertThat(response.email()).isEqualTo(email);
    }

    @Test
    void refreshAccessToken_expiredToken_throwsException() {
        String refreshTokenStr = UUID.randomUUID().toString();
        User user = new User("user@example.com", "User", null);
        // expired: expiresAt is in the past
        RefreshToken expiredToken = new RefreshToken(
                user, refreshTokenStr, Instant.now().minusSeconds(1));

        when(refreshTokenRepository.findByToken(refreshTokenStr)).thenReturn(Optional.of(expiredToken));

        assertThatThrownBy(() -> authService.refreshAccessToken(refreshTokenStr))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("expired");
    }

    @Test
    void refreshAccessToken_tokenNotFound_throwsException() {
        String refreshTokenStr = UUID.randomUUID().toString();

        when(refreshTokenRepository.findByToken(refreshTokenStr)).thenReturn(Optional.empty());

        assertThatThrownBy(() -> authService.refreshAccessToken(refreshTokenStr))
                .isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void register_newUser_createsUserAndReturnsTokens() {
        String email = "new@example.com";
        String name = "New User";
        String password = "password123";

        when(userRepository.findByEmail(email)).thenReturn(Optional.empty());
        when(userRepository.save(any(User.class))).thenAnswer(inv -> inv.getArgument(0));
        when(jwtService.generateAccessToken(any(), eq(email))).thenReturn("access-token");
        when(jwtService.generateRefreshTokenString()).thenReturn("refresh-token");
        when(jwtService.getRefreshTokenTtlMs()).thenReturn(604_800_000L);
        when(refreshTokenRepository.save(any())).thenAnswer(inv -> inv.getArgument(0));

        AuthResponse response = authService.register(email, password, name);

        assertThat(response.accessToken()).isEqualTo("access-token");
        assertThat(response.email()).isEqualTo(email);
        assertThat(response.name()).isEqualTo(name);
        verify(userRepository).save(argThat(u ->
                u.getEmail().equals(email) && u.getPasswordHash() != null));
    }

    @Test
    void register_existingEmail_throwsException() {
        String email = "existing@example.com";
        when(userRepository.findByEmail(email)).thenReturn(Optional.of(new User(email, "User", null)));

        assertThatThrownBy(() -> authService.register(email, "password123", "User"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("already registered");
    }

    @Test
    void login_validCredentials_returnsTokens() {
        String email = "user@example.com";
        String password = "password123";
        String hashedPassword = passwordEncoder.encode(password);

        User user = new User(email, "User", hashedPassword, true);
        when(userRepository.findByEmail(email)).thenReturn(Optional.of(user));
        when(jwtService.generateAccessToken(any(), eq(email))).thenReturn("access-token");
        when(jwtService.generateRefreshTokenString()).thenReturn("refresh-token");
        when(jwtService.getRefreshTokenTtlMs()).thenReturn(604_800_000L);
        when(refreshTokenRepository.save(any())).thenAnswer(inv -> inv.getArgument(0));

        AuthResponse response = authService.login(email, password);

        assertThat(response.accessToken()).isEqualTo("access-token");
        assertThat(response.email()).isEqualTo(email);
    }

    @Test
    void login_wrongPassword_throwsException() {
        String email = "user@example.com";
        User user = new User(email, "User", passwordEncoder.encode("correct"), true);
        when(userRepository.findByEmail(email)).thenReturn(Optional.of(user));

        assertThatThrownBy(() -> authService.login(email, "wrong"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Invalid");
    }

    @Test
    void login_noAccount_throwsException() {
        when(userRepository.findByEmail("nobody@example.com")).thenReturn(Optional.empty());

        assertThatThrownBy(() -> authService.login("nobody@example.com", "password"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Invalid");
    }

    @Test
    void forgotPassword_existingUser_sendsEmail() {
        String email = "user@example.com";
        User user = new User(email, "User", "hash", true);
        when(userRepository.findByEmail(email)).thenReturn(Optional.of(user));
        when(passwordResetTokenRepository.save(any())).thenAnswer(inv -> inv.getArgument(0));

        authService.forgotPassword(email);

        verify(emailService).sendPasswordResetEmail(eq(email), any(String.class));
    }

    @Test
    void forgotPassword_nonExistentUser_doesNotThrow() {
        when(userRepository.findByEmail("nobody@example.com")).thenReturn(Optional.empty());

        authService.forgotPassword("nobody@example.com");

        verify(emailService, never()).sendPasswordResetEmail(any(), any());
    }

    @Test
    void resetPassword_validToken_updatesPassword() {
        String tokenStr = UUID.randomUUID().toString();
        User user = new User("user@example.com", "User", "old-hash", true);
        PasswordResetToken resetToken = new PasswordResetToken(
                tokenStr, user, Instant.now().plusSeconds(3600));

        when(passwordResetTokenRepository.findByToken(tokenStr)).thenReturn(Optional.of(resetToken));

        authService.resetPassword(tokenStr, "newpassword123");

        assertThat(passwordEncoder.matches("newpassword123", user.getPasswordHash())).isTrue();
        verify(passwordResetTokenRepository).delete(resetToken);
    }

    @Test
    void resetPassword_expiredToken_throwsException() {
        String tokenStr = UUID.randomUUID().toString();
        User user = new User("user@example.com", "User", "hash", true);
        PasswordResetToken expiredToken = new PasswordResetToken(
                tokenStr, user, Instant.now().minusSeconds(1));

        when(passwordResetTokenRepository.findByToken(tokenStr)).thenReturn(Optional.of(expiredToken));

        assertThatThrownBy(() -> authService.resetPassword(tokenStr, "newpassword"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("expired");
    }

    @Test
    void resetPassword_invalidToken_throwsException() {
        when(passwordResetTokenRepository.findByToken("bad-token")).thenReturn(Optional.empty());

        assertThatThrownBy(() -> authService.resetPassword("bad-token", "newpassword"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Invalid");
    }
}
