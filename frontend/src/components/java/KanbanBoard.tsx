"use client";

import { useMutation } from "@apollo/client/react";
import { gql } from "@apollo/client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { TaskCard } from "./TaskCard";
import { CreateTaskDialog } from "./CreateTaskDialog";

const UPDATE_TASK = gql`
  mutation UpdateTask($id: ID!, $input: UpdateTaskInput!) {
    updateTask(id: $id, input: $input) {
      id
      status
    }
  }
`;

interface Task {
  id: string;
  projectId: string;
  title: string;
  description: string | null;
  status: string;
  priority: string;
  assigneeId: string | null;
  assigneeName: string | null;
}

interface Props {
  projectId: string;
  tasks: Task[];
  refetch: () => void;
}

const columns = [
  { key: "TODO", label: "To Do" },
  { key: "IN_PROGRESS", label: "In Progress" },
  { key: "DONE", label: "Done" },
] as const;

export function KanbanBoard({ projectId, tasks, refetch }: Props) {
  const [showCreate, setShowCreate] = useState(false);
  const [updateTask] = useMutation(UPDATE_TASK);

  const moveTask = async (taskId: string, newStatus: string) => {
    await updateTask({
      variables: { id: taskId, input: { status: newStatus } },
    });
    refetch();
  };

  return (
    <div>
      <div className="flex items-center justify-between">
        <div />
        <Button onClick={() => setShowCreate(true)}>New Task</Button>
      </div>

      <div className="mt-6 grid grid-cols-3 gap-4">
        {columns.map((col) => {
          const columnTasks = tasks.filter((t) => t.status === col.key);
          return (
            <div
              key={col.key}
              className="rounded-xl border border-foreground/10 bg-card/50 p-4"
            >
              <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
                {col.label}{" "}
                <span className="text-xs">({columnTasks.length})</span>
              </h3>
              <div className="mt-3 space-y-2">
                {columnTasks.map((task) => (
                  <div key={task.id}>
                    <TaskCard task={task} />
                    <div className="mt-1 flex gap-1">
                      {columns
                        .filter((c) => c.key !== task.status)
                        .map((c) => (
                          <button
                            key={c.key}
                            onClick={() => moveTask(task.id, c.key)}
                            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                          >
                            &rarr; {c.label}
                          </button>
                        ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          );
        })}
      </div>

      {showCreate && (
        <CreateTaskDialog
          projectId={projectId}
          onClose={() => setShowCreate(false)}
          onCreated={() => {
            setShowCreate(false);
            refetch();
          }}
        />
      )}
    </div>
  );
}
