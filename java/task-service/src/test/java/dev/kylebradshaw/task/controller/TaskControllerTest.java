package dev.kylebradshaw.task.controller;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.doNothing;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.fasterxml.jackson.databind.ObjectMapper;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.Task;
import dev.kylebradshaw.task.entity.TaskPriority;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.service.TaskService;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.http.MediaType;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

@WebMvcTest(TaskController.class)
class TaskControllerTest {

    @TestConfiguration
    static class TestSecurityConfig {
        @Bean
        public SecurityFilterChain testFilterChain(HttpSecurity http) throws Exception {
            return http.csrf(c -> c.disable())
                    .authorizeHttpRequests(a -> a.anyRequest().permitAll())
                    .build();
        }
    }

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockitoBean
    private TaskService taskService;

    private Task buildTask(String title) {
        User owner = new User("owner@example.com", "Owner Name", null);
        Project project = new Project("Test Project", "desc", owner);
        return new Task(project, title, "desc", TaskPriority.MEDIUM, null);
    }

    @Test
    void createTask_returns201() throws Exception {
        UUID projectId = UUID.randomUUID();
        UUID actorId = UUID.randomUUID();
        Task task = buildTask("Write tests");

        when(taskService.createTask(any(), eq(actorId))).thenReturn(task);

        String body = objectMapper.writeValueAsString(Map.of(
                "projectId", projectId.toString(),
                "title", "Write tests"
        ));

        mockMvc.perform(post("/tasks")
                        .header("X-User-Id", actorId.toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.title").value("Write tests"));
    }

    @Test
    void getTasksByProject_returns200() throws Exception {
        UUID projectId = UUID.randomUUID();
        Task task = buildTask("Implement feature");

        when(taskService.getTasksByProject(projectId)).thenReturn(List.of(task));

        mockMvc.perform(get("/tasks")
                        .param("projectId", projectId.toString()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].title").value("Implement feature"));
    }

    @Test
    void getTask_returns200() throws Exception {
        UUID taskId = UUID.randomUUID();
        Task task = buildTask("Get task");

        when(taskService.getTask(taskId)).thenReturn(task);

        mockMvc.perform(get("/tasks/{id}", taskId))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.title").value("Get task"));
    }

    @Test
    void updateTask_returns200() throws Exception {
        UUID taskId = UUID.randomUUID();
        UUID actorId = UUID.randomUUID();
        Task updated = buildTask("Updated Title");

        when(taskService.updateTask(eq(taskId), any(), eq(actorId))).thenReturn(updated);

        String body = objectMapper.writeValueAsString(Map.of("title", "Updated Title"));

        mockMvc.perform(put("/tasks/{id}", taskId)
                        .header("X-User-Id", actorId.toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.title").value("Updated Title"));
    }

    @Test
    void assignTask_returns200() throws Exception {
        UUID taskId = UUID.randomUUID();
        UUID assigneeId = UUID.randomUUID();
        UUID actorId = UUID.randomUUID();
        Task assigned = buildTask("Assigned Task");

        when(taskService.assignTask(taskId, assigneeId, actorId)).thenReturn(assigned);

        mockMvc.perform(put("/tasks/{id}/assign/{assigneeId}", taskId, assigneeId)
                        .header("X-User-Id", actorId.toString()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.title").value("Assigned Task"));
    }

    @Test
    void deleteTask_returns204() throws Exception {
        UUID taskId = UUID.randomUUID();
        UUID actorId = UUID.randomUUID();

        doNothing().when(taskService).deleteTask(taskId, actorId);

        mockMvc.perform(delete("/tasks/{id}", taskId)
                        .header("X-User-Id", actorId.toString()))
                .andExpect(status().isNoContent());
    }
}
