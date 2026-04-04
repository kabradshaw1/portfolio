"use client";

import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { TaskDetail } from "@/components/java/TaskDetail";

export default function TaskPage() {
  const params = useParams();
  const router = useRouter();
  const projectId = params.projectId as string;
  const taskId = params.taskId as string;

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link
        href={`/java/tasks/${projectId}`}
        className="text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        &larr; Back to board
      </Link>
      <div className="mt-6">
        <TaskDetail
          taskId={taskId}
          onDeleted={() => router.push(`/java/tasks/${projectId}`)}
        />
      </div>
    </div>
  );
}
