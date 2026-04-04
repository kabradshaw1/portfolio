package dev.kylebradshaw.activity.listener;

import dev.kylebradshaw.activity.config.RabbitConfig;
import dev.kylebradshaw.activity.document.ActivityEvent;
import dev.kylebradshaw.activity.dto.TaskEventMessage;
import dev.kylebradshaw.activity.repository.ActivityEventRepository;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.stereotype.Component;

@Component
public class TaskEventListener {
    private final ActivityEventRepository activityRepo;

    public TaskEventListener(ActivityEventRepository activityRepo) {
        this.activityRepo = activityRepo;
    }

    @RabbitListener(queues = RabbitConfig.QUEUE_NAME)
    public void handleTaskEvent(TaskEventMessage message) {
        var event = new ActivityEvent(
                message.projectId().toString(), message.taskId().toString(),
                message.actorId().toString(), message.eventType(), message.data());
        activityRepo.save(event);
    }
}
