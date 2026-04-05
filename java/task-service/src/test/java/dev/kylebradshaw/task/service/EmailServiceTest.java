package dev.kylebradshaw.task.service;

import com.resend.Resend;
import com.resend.core.exception.ResendException;
import com.resend.services.emails.Emails;
import com.resend.services.emails.model.CreateEmailOptions;
import com.resend.services.emails.model.CreateEmailResponse;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class EmailServiceTest {

    @Mock
    private Resend resend;

    @Mock
    private Emails emails;

    private EmailService emailService;

    @BeforeEach
    void setUp() {
        when(resend.emails()).thenReturn(emails);
        emailService = new EmailService(resend, "noreply@resend.dev", "https://kylebradshaw.dev");
    }

    @Test
    void sendPasswordResetEmail_sendsWithCorrectParams() throws ResendException {
        CreateEmailResponse mockResponse = new CreateEmailResponse();
        when(emails.send(any(CreateEmailOptions.class))).thenReturn(mockResponse);

        emailService.sendPasswordResetEmail("user@example.com", "reset-token-123");

        ArgumentCaptor<CreateEmailOptions> captor = ArgumentCaptor.forClass(CreateEmailOptions.class);
        verify(emails).send(captor.capture());

        CreateEmailOptions options = captor.getValue();
        assertThat(options.getFrom()).isEqualTo("noreply@resend.dev");
        assertThat(options.getSubject()).isEqualTo("Reset your password");
    }
}
