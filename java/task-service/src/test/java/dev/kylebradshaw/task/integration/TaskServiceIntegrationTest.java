package dev.kylebradshaw.task.integration;

import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.fasterxml.jackson.databind.ObjectMapper;
import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.Map;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.test.web.servlet.MockMvc;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.containers.RabbitMQContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Tag("integration")
@SpringBootTest
@AutoConfigureMockMvc
@Testcontainers
class TaskServiceIntegrationTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:17-alpine")
            .withDatabaseName("taskdb").withUsername("test").withPassword("test");

    @Container
    static RabbitMQContainer rabbitmq = new RabbitMQContainer("rabbitmq:3-management-alpine");

    @DynamicPropertySource
    static void configureProperties(DynamicPropertyRegistry registry) {
        registry.add("spring.datasource.url", postgres::getJdbcUrl);
        registry.add("spring.datasource.username", postgres::getUsername);
        registry.add("spring.datasource.password", postgres::getPassword);
        registry.add("spring.rabbitmq.host", rabbitmq::getHost);
        registry.add("spring.rabbitmq.port", rabbitmq::getAmqpPort);
        registry.add("app.jwt.secret", () -> "integration-test-secret-key-at-least-32-characters");
        registry.add("app.jwt.access-token-ttl-ms", () -> "900000");
        registry.add("app.jwt.refresh-token-ttl-ms", () -> "604800000");
        registry.add("app.allowed-origins", () -> "http://localhost:3000");
        registry.add("app.google.client-id", () -> "test-client-id");
        registry.add("app.google.client-secret", () -> "test-client-secret");
    }

    @Autowired private MockMvc mockMvc;
    @Autowired private ObjectMapper objectMapper;
    @Autowired private UserRepository userRepo;

    private User testUser;

    @BeforeEach
    void setUp() {
        testUser = userRepo.findByEmail("integration@test.com")
                .orElseGet(() -> userRepo.save(
                        new User("integration@test.com", "Integration User", null)));
    }

    @Test
    void createAndGetProject() throws Exception {
        var request = new CreateProjectRequest("Integration Project", "Testing");
        mockMvc.perform(post("/api/projects")
                        .header("X-User-Id", testUser.getId().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(request)))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.name").value("Integration Project"));

        mockMvc.perform(get("/api/projects")
                        .header("X-User-Id", testUser.getId().toString()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[?(@.name == 'Integration Project')]").exists());
    }

    @Test
    void createTask_viaProject() throws Exception {
        var projectReq = new CreateProjectRequest("Task Test Project", "For tasks");
        String projectJson = mockMvc.perform(post("/api/projects")
                        .header("X-User-Id", testUser.getId().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(projectReq)))
                .andExpect(status().isCreated())
                .andReturn().getResponse().getContentAsString();

        String projectId = objectMapper.readTree(projectJson).get("id").asText();

        mockMvc.perform(post("/api/tasks")
                        .header("X-User-Id", testUser.getId().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(Map.of(
                                "projectId", projectId, "title", "Integration Task",
                                "priority", "HIGH"))))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.title").value("Integration Task"))
                .andExpect(jsonPath("$.status").value("TODO"));
    }
}
