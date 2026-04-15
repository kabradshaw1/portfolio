package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.ProjectMember;
import dev.kylebradshaw.task.entity.ProjectRole;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.ProjectMemberRepository;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.List;
import java.util.UUID;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class ProjectService {
    private final ProjectRepository projectRepo;
    private final ProjectMemberRepository memberRepo;
    private final UserRepository userRepo;

    public ProjectService(ProjectRepository projectRepo, ProjectMemberRepository memberRepo, UserRepository userRepo) {
        this.projectRepo = projectRepo;
        this.memberRepo = memberRepo;
        this.userRepo = userRepo;
    }

    @Transactional
    public Project createProject(CreateProjectRequest request, UUID userId) {
        User owner = userRepo.findById(userId).orElseThrow(() -> new IllegalArgumentException("User not found"));
        Project project = new Project(request.name(), request.description(), owner);
        project = projectRepo.save(project);
        var member = new ProjectMember(project.getId(), userId, ProjectRole.OWNER);
        memberRepo.save(member);
        return project;
    }

    public List<Project> getProjectsForUser(UUID userId) {
        List<UUID> projectIds = memberRepo.findByUserId(userId).stream().map(ProjectMember::getProjectId).toList();
        if (projectIds.isEmpty()) {
            return List.of();
        }
        return projectRepo.findAllByIdWithOwner(projectIds);
    }

    public Project getProject(UUID projectId, UUID userId) {
        Project project = projectRepo.findByIdWithOwner(projectId)
                .orElseThrow(() -> new IllegalArgumentException("Project not found"));
        if (!memberRepo.existsByProjectIdAndUserId(projectId, userId)) {
            throw new IllegalArgumentException("Project not found");
        }
        return project;
    }

    @Transactional
    public Project updateProject(UUID projectId, UUID userId, String name, String description) {
        if (!memberRepo.existsByProjectIdAndUserIdAndRole(projectId, userId, ProjectRole.OWNER)) {
            throw new IllegalStateException("Only the owner can update the project");
        }
        Project project = projectRepo.findByIdWithOwner(projectId)
                .orElseThrow(() -> new IllegalArgumentException("Project not found"));
        if (name != null) {
            project.setName(name);
        }
        if (description != null) {
            project.setDescription(description);
        }
        return projectRepo.save(project);
    }

    @Transactional
    public void deleteProject(UUID projectId, UUID userId) {
        if (!memberRepo.existsByProjectIdAndUserIdAndRole(projectId, userId, ProjectRole.OWNER)) {
            throw new IllegalStateException("Only the owner can delete the project");
        }
        projectRepo.deleteById(projectId);
    }
}
