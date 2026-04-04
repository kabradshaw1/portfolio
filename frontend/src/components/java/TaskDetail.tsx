"use client";

import { useQuery, useMutation } from "@apollo/client/react";
import { gql } from "@apollo/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CommentSection } from "./CommentSection";
import { ActivityTimeline } from "./ActivityTimeline";

const GET_TASK = gql`
  query GetTask($id: ID!) {
    task(id: $id) {
      id
      projectId
      title
      description
      status
      priority
      assigneeName
      dueDate
      createdAt
      updatedAt
    }
  }
`;

const UPDATE_TASK = gql`
  mutation UpdateTask($id: ID!, $input: UpdateTaskInput!) {
    updateTask(id: $id, input: $input) {
      id
      status
    }
  }
`;

const DELETE_TASK = gql`
  mutation DeleteTask($id: ID!) {
    deleteTask(id: $id)
  }
`;

const statusOptions = ["TODO", "IN_PROGRESS", "DONE"];
const priorityColors: Record<string, string> = {
  HIGH: "bg-red-500/10 text-red-500",
  MEDIUM: "bg-yellow-500/10 text-yellow-500",
  LOW: "bg-green-500/10 text-green-500",
};

interface Props {
  taskId: string;
  onDeleted: () => void;
}

export function TaskDetail({ taskId, onDeleted }: Props) {
  const { data, loading, refetch } = useQuery(GET_TASK, {
    variables: { id: taskId },
  });
  const [updateTask] = useMutation(UPDATE_TASK);
  const [deleteTask] = useMutation(DELETE_TASK);

  if (loading) return <p className="text-muted-foreground">Loading...</p>;

  const task = (data as { task?: { id: string; projectId: string; title: string; description?: string; status: string; priority: string; assigneeName?: string; dueDate?: string; createdAt: string; updatedAt: string } } | undefined)?.task;
  if (!task) return <p className="text-muted-foreground">Task not found.</p>;

  const handleStatusChange = async (status: string) => {
    await updateTask({ variables: { id: taskId, input: { status } } });
    refetch();
  };

  const handleDelete = async () => {
    await deleteTask({ variables: { id: taskId } });
    onDeleted();
  };

  return (
    <div className="space-y-8">
      <div>
        <div className="flex items-start justify-between">
          <h1 className="text-2xl font-bold">{task.title}</h1>
          <Button variant="destructive" size="sm" onClick={handleDelete}>
            Delete
          </Button>
        </div>
        {task.description && (
          <p className="mt-2 text-muted-foreground">{task.description}</p>
        )}
      </div>

      <div className="flex flex-wrap gap-3">
        <Badge
          variant="secondary"
          className={priorityColors[task.priority] || ""}
        >
          {task.priority}
        </Badge>
        {task.assigneeName && (
          <Badge variant="outline">Assigned: {task.assigneeName}</Badge>
        )}
        {task.dueDate && (
          <Badge variant="outline">Due: {task.dueDate}</Badge>
        )}
      </div>

      <div>
        <h3 className="text-sm font-medium text-muted-foreground">Status</h3>
        <div className="mt-2 flex gap-2">
          {statusOptions.map((s) => (
            <Button
              key={s}
              variant={task.status === s ? "default" : "outline"}
              size="sm"
              onClick={() => handleStatusChange(s)}
            >
              {s.replace("_", " ")}
            </Button>
          ))}
        </div>
      </div>

      <CommentSection taskId={taskId} />
      <ActivityTimeline taskId={taskId} />
    </div>
  );
}
