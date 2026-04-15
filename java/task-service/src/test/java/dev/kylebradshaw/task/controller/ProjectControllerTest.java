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
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.service.ProjectService;
import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.http.MediaType;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.test.context.support.WithMockUser;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

@WebMvcTest(ProjectController.class)
class ProjectControllerTest {

    static final String TEST_USER_ID = "00000000-0000-0000-0000-000000000001";

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
    private ProjectService projectService;

    @Test
    @WithMockUser(username = TEST_USER_ID)
    void createProject_returns201WithProjectName() throws Exception {
        User owner = new User("owner@example.com", "Owner Name", null);
        Project project = new Project("My Project", "A description", owner);

        when(projectService.createProject(any(), any())).thenReturn(project);

        String body = objectMapper.writeValueAsString(
                new java.util.HashMap<String, String>() {{
                    put("name", "My Project");
                    put("description", "A description");
                }}
        );

        mockMvc.perform(post("/projects")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.name").value("My Project"));
    }

    @Test
    @WithMockUser(username = TEST_USER_ID)
    void getMyProjects_returns200WithList() throws Exception {
        User owner = new User("owner@example.com", "Owner Name", null);
        Project project = new Project("Alpha Project", "desc", owner);

        when(projectService.getProjectsForUser(any())).thenReturn(List.of(project));

        mockMvc.perform(get("/projects"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].name").value("Alpha Project"));
    }

    @Test
    @WithMockUser(username = TEST_USER_ID)
    void getProject_returns200() throws Exception {
        UUID projectId = UUID.randomUUID();
        User owner = new User("owner@example.com", "Owner Name", null);
        Project project = new Project("Beta Project", "desc", owner);

        when(projectService.getProject(eq(projectId), any())).thenReturn(project);

        mockMvc.perform(get("/projects/{id}", projectId))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.name").value("Beta Project"));
    }

    @Test
    @WithMockUser(username = TEST_USER_ID)
    void updateProject_returns200() throws Exception {
        UUID projectId = UUID.randomUUID();
        User owner = new User("owner@example.com", "Owner Name", null);
        Project updated = new Project("Updated Name", "updated desc", owner);

        when(projectService.updateProject(eq(projectId), any(), any(), any())).thenReturn(updated);

        String body = objectMapper.writeValueAsString(
                new java.util.HashMap<String, String>() {{
                    put("name", "Updated Name");
                    put("description", "updated desc");
                }}
        );

        mockMvc.perform(put("/projects/{id}", projectId)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.name").value("Updated Name"));
    }

    @Test
    @WithMockUser(username = TEST_USER_ID)
    void deleteProject_returns204() throws Exception {
        UUID projectId = UUID.randomUUID();

        doNothing().when(projectService).deleteProject(eq(projectId), any());

        mockMvc.perform(delete("/projects/{id}", projectId))
                .andExpect(status().isNoContent());
    }
}
