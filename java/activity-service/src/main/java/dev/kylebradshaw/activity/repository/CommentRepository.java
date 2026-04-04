package dev.kylebradshaw.activity.repository;

import dev.kylebradshaw.activity.document.Comment;
import java.util.List;
import org.springframework.data.mongodb.repository.MongoRepository;

public interface CommentRepository extends MongoRepository<Comment, String> {
    List<Comment> findByTaskIdOrderByCreatedAtAsc(String taskId);
}
