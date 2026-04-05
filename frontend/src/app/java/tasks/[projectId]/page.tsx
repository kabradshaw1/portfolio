"use client";

import { useParams } from "next/navigation";
import { useQuery } from "@apollo/client/react";
import { gql } from "@apollo/client";
import Link from "next/link";
import { KanbanBoard } from "@/components/java/KanbanBoard";
import { GATEWAY_URL, getAccessToken } from "@/lib/auth";
import { useEffect, useState, useCallback } from "react";

const GET_PROJECT = gql`
  query GetProject($id: ID!) {
    project(id: $id) {
      id
      name
      description
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

interface ProjectData {
  project: {
    id: string;
    name: string;
    description: string | null;
  };
}

export default function ProjectPage() {
  const params = useParams();
  const projectId = params.projectId as string;
  const { data: projectData, loading: projectLoading } = useQuery<ProjectData>(GET_PROJECT, {
    variables: { id: projectId },
  });
  const [tasks, setTasks] = useState<Task[]>([]);
  const [tasksLoading, setTasksLoading] = useState(true);

  const loadTasks = useCallback(async (): Promise<Task[]> => {
    const token = getAccessToken();
    const res = await fetch(
      `${GATEWAY_URL}/api/tasks?projectId=${projectId}`,
      {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      }
    );
    return res.ok ? res.json() : [];
  }, [projectId]);

  const fetchTasks = useCallback(() => {
    loadTasks().then((data) => {
      setTasks(data);
      setTasksLoading(false);
    });
  }, [loadTasks]);

  useEffect(() => {
    loadTasks().then((data) => {
      setTasks(data);
      setTasksLoading(false);
    });
  }, [loadTasks]);

  if (projectLoading || tasksLoading) {
    return (
      <div className="mx-auto max-w-5xl px-6 py-12">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  const project = projectData?.project;

  return (
    <div className="mx-auto max-w-5xl px-6 py-12">
      <Link
        href="/java/tasks"
        className="text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        &larr; Projects
      </Link>
      <h1 className="mt-4 text-2xl font-bold">{project?.name}</h1>
      {project?.description && (
        <p className="mt-1 text-muted-foreground">{project.description}</p>
      )}
      <div className="mt-8">
        <KanbanBoard
          projectId={projectId}
          tasks={tasks}
          refetch={fetchTasks}
        />
      </div>
    </div>
  );
}
