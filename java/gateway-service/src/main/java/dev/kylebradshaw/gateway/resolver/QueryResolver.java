package dev.kylebradshaw.gateway.resolver;

import dev.kylebradshaw.gateway.client.ActivityServiceClient;
import dev.kylebradshaw.gateway.client.NotificationServiceClient;
import dev.kylebradshaw.gateway.client.TaskServiceClient;
import dev.kylebradshaw.gateway.dto.ActivityEventDto;
import dev.kylebradshaw.gateway.dto.CommentDto;
import dev.kylebradshaw.gateway.dto.NotificationDto;
import dev.kylebradshaw.gateway.dto.ProjectDto;
import dev.kylebradshaw.gateway.dto.TaskDto;
import graphql.schema.DataFetchingEnvironment;
import org.springframework.graphql.data.method.annotation.Argument;
import org.springframework.graphql.data.method.annotation.QueryMapping;
import org.springframework.stereotype.Controller;

import java.util.List;

@Controller
public class QueryResolver {

    private final TaskServiceClient taskClient;
    private final ActivityServiceClient activityClient;
    private final NotificationServiceClient notificationClient;

    public QueryResolver(TaskServiceClient taskClient,
                         ActivityServiceClient activityClient,
                         NotificationServiceClient notificationClient) {
        this.taskClient = taskClient;
        this.activityClient = activityClient;
        this.notificationClient = notificationClient;
    }

    @QueryMapping
    public List<ProjectDto> myProjects(DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return taskClient.getMyProjects(authHeader);
    }

    @QueryMapping
    public ProjectDto project(@Argument String id, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return taskClient.getProject(id, authHeader);
    }

    @QueryMapping
    public TaskDto task(@Argument String id, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return taskClient.getTask(id, authHeader);
    }

    @QueryMapping
    public List<ActivityEventDto> taskActivity(@Argument String taskId, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return activityClient.getActivityByTask(taskId, authHeader);
    }

    @QueryMapping
    public List<CommentDto> taskComments(@Argument String taskId, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return activityClient.getCommentsByTask(taskId, authHeader);
    }

    @QueryMapping
    public NotificationDto myNotifications(@Argument Boolean unreadOnly, DataFetchingEnvironment env) {
        String authHeader = env.getGraphQlContext().get("authorizationHeader");
        return notificationClient.getNotifications(authHeader, unreadOnly);
    }
}
