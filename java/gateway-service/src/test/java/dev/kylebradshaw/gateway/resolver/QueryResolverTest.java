package dev.kylebradshaw.gateway.resolver;

import dev.kylebradshaw.gateway.client.ActivityServiceClient;
import dev.kylebradshaw.gateway.client.NotificationServiceClient;
import dev.kylebradshaw.gateway.client.TaskServiceClient;
import dev.kylebradshaw.gateway.dto.ProjectDto;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.graphql.GraphQlTest;
import org.springframework.graphql.test.tester.ExecutionGraphQlServiceTester;
import org.springframework.graphql.test.tester.GraphQlTester;
import org.springframework.test.context.bean.override.mockito.MockitoBean;

import java.util.List;

import static org.mockito.Mockito.when;

/**
 * Tests the QueryResolver using @GraphQlTest with ExecutionGraphQlServiceTester.
 * The userId is injected into the GraphQL context via configureExecutionInput,
 * bypassing the WebGraphQlInterceptor (which handles the real HTTP flow).
 */
@GraphQlTest(QueryResolver.class)
class QueryResolverTest {

    @Autowired
    private GraphQlTester graphQlTester;

    @MockitoBean
    private TaskServiceClient taskClient;

    @MockitoBean
    private ActivityServiceClient activityClient;

    @MockitoBean
    private NotificationServiceClient notificationClient;

    @Test
    void myProjects_returnsProjects() {
        String testAuthHeader = "Bearer test-token";
        when(taskClient.getMyProjects(testAuthHeader))
                .thenReturn(List.of(new ProjectDto("1", "Project 1", "Desc", "o1", "Owner", "2026-04-03T00:00:00Z")));

        // Inject authorizationHeader into the GraphQL execution context
        ExecutionGraphQlServiceTester tester = ((ExecutionGraphQlServiceTester) graphQlTester)
                .mutate()
                .configureExecutionInput((input, builder) ->
                        builder.graphQLContext(ctx -> {
                            ctx.put("userId", "test-user");
                            ctx.put("authorizationHeader", testAuthHeader);
                        }).build())
                .build();

        tester.document("query { myProjects { id name } }")
                .execute()
                .path("myProjects[0].name").entity(String.class).isEqualTo("Project 1");
    }
}
