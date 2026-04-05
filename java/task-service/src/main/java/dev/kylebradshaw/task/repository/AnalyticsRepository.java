package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.dto.MemberWorkloadRow;
import dev.kylebradshaw.task.dto.PercentilesRow;
import dev.kylebradshaw.task.dto.WeeklyThroughputRow;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.stereotype.Repository;

@Repository
public class AnalyticsRepository {

    private final JdbcClient jdbc;

    public AnalyticsRepository(JdbcClient jdbc) {
        this.jdbc = jdbc;
    }

    public Map<String, Integer> countByStatus(UUID projectId) {
        var rows = jdbc.sql("""
                SELECT status, COUNT(*) AS cnt
                FROM tasks
                WHERE project_id = :projectId
                GROUP BY status
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> Map.entry(rs.getString("status"), rs.getInt("cnt")))
                .list();
        return Map.ofEntries(rows.toArray(Map.Entry[]::new));
    }

    public Map<String, Integer> countByPriority(UUID projectId) {
        var rows = jdbc.sql("""
                SELECT priority, COUNT(*) AS cnt
                FROM tasks
                WHERE project_id = :projectId
                GROUP BY priority
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> Map.entry(rs.getString("priority"), rs.getInt("cnt")))
                .list();
        return Map.ofEntries(rows.toArray(Map.Entry[]::new));
    }

    public int countOverdue(UUID projectId) {
        return jdbc.sql("""
                SELECT COUNT(*) AS cnt
                FROM tasks
                WHERE project_id = :projectId
                  AND status != 'DONE'
                  AND due_date < CURRENT_DATE
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> rs.getInt("cnt"))
                .single();
    }

    public Double avgCompletionTimeHours(UUID projectId) {
        return jdbc.sql("""
                SELECT AVG(EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600) AS avg_hours
                FROM tasks
                WHERE project_id = :projectId
                  AND completed_at IS NOT NULL
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> {
                    double val = rs.getDouble("avg_hours");
                    return rs.wasNull() ? null : val;
                })
                .single();
    }

    public List<MemberWorkloadRow> memberWorkload(UUID projectId) {
        return jdbc.sql("""
                SELECT u.id AS user_id,
                       u.name,
                       COUNT(*) FILTER (WHERE t.status != 'DONE') AS assigned_count,
                       COUNT(*) FILTER (WHERE t.status = 'DONE') AS completed_count
                FROM tasks t
                JOIN users u ON u.id = t.assignee_id
                WHERE t.project_id = :projectId
                  AND t.assignee_id IS NOT NULL
                GROUP BY u.id, u.name
                ORDER BY assigned_count DESC
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> new MemberWorkloadRow(
                        rs.getObject("user_id", UUID.class),
                        rs.getString("name"),
                        rs.getInt("assigned_count"),
                        rs.getInt("completed_count")))
                .list();
    }

    public List<WeeklyThroughputRow> weeklyThroughput(UUID projectId, int weeks) {
        return jdbc.sql("""
                WITH week_series AS (
                    SELECT generate_series(
                        date_trunc('week', now()) - ((:weeks - 1) * INTERVAL '1 week'),
                        date_trunc('week', now()),
                        INTERVAL '1 week'
                    ) AS week_start
                )
                SELECT to_char(ws.week_start, 'IYYY-"W"IW') AS week,
                       COALESCE(SUM(CASE WHEN t.completed_at >= ws.week_start
                                          AND t.completed_at < ws.week_start + INTERVAL '1 week'
                                         THEN 1 ELSE 0 END), 0) AS completed,
                       COALESCE(SUM(CASE WHEN t.created_at >= ws.week_start
                                          AND t.created_at < ws.week_start + INTERVAL '1 week'
                                         THEN 1 ELSE 0 END), 0) AS created
                FROM week_series ws
                LEFT JOIN tasks t ON t.project_id = :projectId
                GROUP BY ws.week_start
                ORDER BY ws.week_start DESC
                """)
                .param("projectId", projectId)
                .param("weeks", weeks)
                .query((rs, rowNum) -> new WeeklyThroughputRow(
                        rs.getString("week"),
                        rs.getInt("completed"),
                        rs.getInt("created")))
                .list();
    }

    public PercentilesRow leadTimePercentiles(UUID projectId) {
        return jdbc.sql("""
                SELECT COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP
                           (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p50,
                       COALESCE(PERCENTILE_CONT(0.75) WITHIN GROUP
                           (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p75,
                       COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP
                           (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p95
                FROM tasks
                WHERE project_id = :projectId
                  AND completed_at IS NOT NULL
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> new PercentilesRow(
                        rs.getDouble("p50"),
                        rs.getDouble("p75"),
                        rs.getDouble("p95")))
                .single();
    }
}
