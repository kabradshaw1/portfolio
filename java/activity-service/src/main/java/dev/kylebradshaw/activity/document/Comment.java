package dev.kylebradshaw.activity.document;

import java.time.Instant;
import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

@Document(collection = "comments")
public class Comment {
    @Id private String id;
    private String taskId;
    private String authorId;
    private String body;
    private Instant createdAt;

    public Comment() {}

    public Comment(String taskId, String authorId, String body) {
        this.taskId = taskId;
        this.authorId = authorId;
        this.body = body;
        this.createdAt = Instant.now();
    }

    public String getId() { return id; }
    public String getTaskId() { return taskId; }
    public String getAuthorId() { return authorId; }
    public String getBody() { return body; }
    public Instant getCreatedAt() { return createdAt; }
}
