package dev.kylebradshaw.activity.repository;

import dev.kylebradshaw.activity.document.ActivityEvent;
import java.util.List;
import org.springframework.data.mongodb.repository.MongoRepository;

public interface ActivityEventRepository extends MongoRepository<ActivityEvent, String> {
    List<ActivityEvent> findByTaskIdOrderByTimestampDesc(String taskId);
    List<ActivityEvent> findByProjectIdOrderByTimestampDesc(String projectId);
}
