package dev.kylebradshaw.activity.document;

import java.time.Instant;
import java.util.Map;
import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

@Document(collection = "activity_events")
public class ActivityEvent {
    @Id private String id;
    private String projectId;
    private String taskId;
    private String actorId;
    private String eventType;
    private Map<String, Object> metadata;
    private Instant timestamp;

    public ActivityEvent() {}

    public ActivityEvent(String projectId, String taskId, String actorId, String eventType, Map<String, Object> metadata) {
        this.projectId = projectId;
        this.taskId = taskId;
        this.actorId = actorId;
        this.eventType = eventType;
        this.metadata = metadata;
        this.timestamp = Instant.now();
    }

    public String getId() { return id; }
    public String getProjectId() { return projectId; }
    public String getTaskId() { return taskId; }
    public String getActorId() { return actorId; }
    public String getEventType() { return eventType; }
    public Map<String, Object> getMetadata() { return metadata; }
    public Instant getTimestamp() { return timestamp; }
}
