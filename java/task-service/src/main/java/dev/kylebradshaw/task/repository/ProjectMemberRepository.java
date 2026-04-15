package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.entity.ProjectMember;
import dev.kylebradshaw.task.entity.ProjectRole;
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface ProjectMemberRepository extends JpaRepository<ProjectMember, ProjectMember.ProjectMemberId> {
    List<ProjectMember> findByUserId(UUID userId);
    Optional<ProjectMember> findByProjectIdAndUserId(UUID projectId, UUID userId);
    boolean existsByProjectIdAndUserId(UUID projectId, UUID userId);
    boolean existsByProjectIdAndUserIdAndRole(UUID projectId, UUID userId, ProjectRole role);
    void deleteByUserId(UUID userId);
}
