package dev.kylebradshaw.task.service;

import com.resend.Resend;
import com.resend.core.exception.ResendException;
import com.resend.services.emails.model.CreateEmailOptions;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

@Service
public class EmailService {

    private static final Logger log = LoggerFactory.getLogger(EmailService.class);

    private final Resend resend;
    private final String fromEmail;
    private final String frontendUrl;

    public EmailService(
            Resend resend,
            @Value("${app.resend.from-email:onboarding@resend.dev}") String fromEmail,
            @Value("${app.frontend-url:http://localhost:3000}") String frontendUrl) {
        this.resend = resend;
        this.fromEmail = fromEmail;
        this.frontendUrl = frontendUrl;
    }

    public void sendPasswordResetEmail(String toEmail, String token) {
        String resetUrl = frontendUrl + "/java/tasks/reset-password?token=" + token;
        String subject = "Reset your password";
        String body = "<h2>Reset your password</h2>"
                + "<p>Click the link below to reset your password. This link expires in 1 hour.</p>"
                + "<p><a href=\"" + resetUrl + "\">Reset Password</a></p>"
                + "<p>If you didn't request this, you can ignore this email.</p>";

        CreateEmailOptions options = CreateEmailOptions.builder()
                .from(fromEmail)
                .to(toEmail)
                .subject(subject)
                .html(body)
                .build();

        try {
            resend.emails().send(options);
        } catch (ResendException e) {
            log.error("Failed to send password reset email to {}", toEmail, e);
            throw new RuntimeException("Failed to send password reset email", e);
        }
    }
}
