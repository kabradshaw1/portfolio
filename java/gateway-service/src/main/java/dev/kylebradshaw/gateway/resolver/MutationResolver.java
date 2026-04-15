package dev.kylebradshaw.gateway.resolver;

import dev.kylebradshaw.gateway.client.ActivityServiceClient;
import dev.kylebradshaw.gateway.client.NotificationServiceClient;
import dev.kylebradshaw.gateway.client.TaskServiceClient;
import dev.kylebradshaw.gateway.dto.CommentDto;
import dev.kylebradshaw.gateway.dto.ProjectDto;
import dev.kylebradshaw.gateway.dto.TaskDto;
import graphql.schema.DataFetchingEnvironment;
import org.springframework.graphql.data.method.annotation.Argument;
import org.springframework.graphql.data.method.annotation.MutationMapping;
import org.springframework.stereotype.Controller;

import java.util.Map;

@Controller
public class MutationResolver {

    private final TaskServiceClient taskClient;
    private final ActivityServiceClient activityClient;
    private final NotificationServiceClient notificationClient;

    public MutationResolver(TaskServiceClient taskClient,
                            ActivityServiceClient activityClient,
                            NotificationServiceClient notificationClient) {
        this.taskClient = taskClient;
        this.activityClient = activityClient;
        this.notificationClient = notificationClient;
    }

    @MutationMapping
    public ProjectDto createProject(@Argument Map<String, Object> input, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return taskClient.createProject(authHeader, input);
    }

    @MutationMapping
    public ProjectDto updateProject(@Argument String id, @Argument Map<String, Object> input, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return taskClient.updateProject(id, authHeader, input);
    }

    @MutationMapping
    public Boolean deleteProject(@Argument String id, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        taskClient.deleteProject(id, authHeader);
        return true;
    }

    @MutationMapping
    public TaskDto createTask(@Argument Map<String, Object> input, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return taskClient.createTask(authHeader, input);
    }

    @MutationMapping
    public TaskDto updateTask(@Argument String id, @Argument Map<String, Object> input, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return taskClient.updateTask(id, authHeader, input);
    }

    @MutationMapping
    public Boolean deleteTask(@Argument String id, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        taskClient.deleteTask(id, authHeader);
        return true;
    }

    @MutationMapping
    public TaskDto assignTask(@Argument String taskId, @Argument String userId, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return taskClient.assignTask(taskId, userId, authHeader);
    }

    @MutationMapping
    public CommentDto addComment(@Argument String taskId, @Argument String body, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return activityClient.addComment(taskId, authHeader, body);
    }

    @MutationMapping
    public Boolean markNotificationRead(@Argument String id, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        notificationClient.markRead(authHeader, id);
        return true;
    }

    @MutationMapping
    public Boolean markAllNotificationsRead(DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        notificationClient.markAllRead(authHeader);
        return true;
    }

    @MutationMapping
    public Boolean deleteAccount(DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        taskClient.deleteUser(authHeader);
        return true;
    }
}
