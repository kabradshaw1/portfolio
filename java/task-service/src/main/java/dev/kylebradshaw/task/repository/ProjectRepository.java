package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.entity.Project;
import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

public interface ProjectRepository extends JpaRepository<Project, UUID> {
    @Query("SELECT p FROM Project p JOIN FETCH p.owner WHERE p.id IN :ids")
    List<Project> findAllByIdWithOwner(@Param("ids") List<UUID> ids);

    @Query("SELECT p FROM Project p JOIN FETCH p.owner WHERE p.id = :id")
    java.util.Optional<Project> findByIdWithOwner(@Param("id") UUID id);
}
