"use client";

import Link from "next/link";
import { Badge } from "@/components/ui/badge";

interface TaskCardProps {
  task: {
    id: string;
    projectId: string;
    title: string;
    priority: string;
    assigneeName: string | null;
    assigneeId: string | null;
  };
}

const priorityColors: Record<string, string> = {
  HIGH: "bg-red-500/10 text-red-500",
  MEDIUM: "bg-yellow-500/10 text-yellow-500",
  LOW: "bg-green-500/10 text-green-500",
};

export function TaskCard({ task }: TaskCardProps) {
  return (
    <Link href={`/java/tasks/${task.projectId}/${task.id}`}>
      <div className="rounded-lg border border-foreground/10 bg-card p-3 hover:ring-1 hover:ring-foreground/20 transition-all cursor-pointer">
        <p className="text-sm font-medium">{task.title}</p>
        <div className="mt-2 flex items-center gap-2">
          <Badge
            variant="secondary"
            className={priorityColors[task.priority] || ""}
          >
            {task.priority}
          </Badge>
          {task.assigneeName && (
            <span className="text-xs text-muted-foreground">
              {task.assigneeName}
            </span>
          )}
        </div>
      </div>
    </Link>
  );
}
