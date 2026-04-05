-- Compound indexes for analytics GROUP BY queries
CREATE INDEX idx_tasks_project_status ON tasks (project_id, status);
CREATE INDEX idx_tasks_project_priority ON tasks (project_id, priority);
CREATE INDEX idx_tasks_project_assignee ON tasks (project_id, assignee_id);

-- Partial index: only completed tasks (smaller, faster for velocity queries)
CREATE INDEX idx_tasks_project_completed_at ON tasks (project_id, completed_at)
    WHERE completed_at IS NOT NULL;

-- Partial index: only open tasks with due dates (for overdue count)
CREATE INDEX idx_tasks_overdue ON tasks (project_id, due_date)
    WHERE status != 'DONE' AND due_date IS NOT NULL;
