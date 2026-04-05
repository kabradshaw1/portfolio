package dev.kylebradshaw.task.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.ProjectMember;
import dev.kylebradshaw.task.entity.ProjectRole;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.ProjectMemberRepository;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class ProjectServiceTest {

    @Mock private ProjectRepository projectRepo;
    @Mock private ProjectMemberRepository memberRepo;
    @Mock private UserRepository userRepo;

    private ProjectService service;

    @BeforeEach
    void setUp() {
        service = new ProjectService(projectRepo, memberRepo, userRepo);
    }

    @Test
    void createProject_savesProjectAndAddsMemberAsOwner() {
        UUID userId = UUID.randomUUID();
        User user = new User("test@example.com", "Test User", null);
        when(userRepo.findById(userId)).thenReturn(Optional.of(user));
        when(projectRepo.save(any(Project.class))).thenAnswer(inv -> inv.getArgument(0));

        var request = new CreateProjectRequest("My Project", "A description");
        Project result = service.createProject(request, userId);

        assertThat(result.getName()).isEqualTo("My Project");
        assertThat(result.getDescription()).isEqualTo("A description");
        assertThat(result.getOwner()).isEqualTo(user);

        ArgumentCaptor<ProjectMember> memberCaptor = ArgumentCaptor.forClass(ProjectMember.class);
        verify(memberRepo).save(memberCaptor.capture());
        assertThat(memberCaptor.getValue().getRole()).isEqualTo(ProjectRole.OWNER);
    }

    @Test
    void getProjectsForUser_returnsProjectsWhereUserIsMember() {
        UUID userId = UUID.randomUUID();
        UUID projectId = UUID.randomUUID();
        var membership = new ProjectMember(projectId, userId, ProjectRole.OWNER);
        when(memberRepo.findByUserId(userId)).thenReturn(List.of(membership));

        User owner = new User("test@example.com", "Test User", null);
        Project project = new Project("Project", "Desc", owner);
        when(projectRepo.findAllByIdWithOwner(List.of(projectId))).thenReturn(List.of(project));

        List<Project> result = service.getProjectsForUser(userId);
        assertThat(result).hasSize(1);
        assertThat(result.getFirst().getName()).isEqualTo("Project");
    }

    @Test
    void deleteProject_whenNotOwner_throws() {
        UUID projectId = UUID.randomUUID();
        UUID userId = UUID.randomUUID();
        when(memberRepo.existsByProjectIdAndUserIdAndRole(projectId, userId, ProjectRole.OWNER))
                .thenReturn(false);

        assertThatThrownBy(() -> service.deleteProject(projectId, userId))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Only the owner");
    }
}
