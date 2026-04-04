package dev.kylebradshaw.activity.listener;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;

import dev.kylebradshaw.activity.dto.TaskEventMessage;
import dev.kylebradshaw.activity.repository.ActivityEventRepository;
import java.time.Instant;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class TaskEventListenerTest {
    @Mock private ActivityEventRepository activityRepo;
    @InjectMocks private TaskEventListener listener;

    @Test
    void handleTaskEvent_savesActivityEvent() {
        var message = new TaskEventMessage(UUID.randomUUID(), "TASK_CREATED", Instant.now(),
                UUID.randomUUID(), UUID.randomUUID(), UUID.randomUUID(), Map.of("task_title", "Fix bug"));
        listener.handleTaskEvent(message);
        verify(activityRepo).save(any());
    }
}
