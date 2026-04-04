import { Suspense } from "react";
import { TasksPageContent } from "@/components/java/TasksPageContent";

export default function TasksPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-3xl px-6 py-12">
          <p className="text-muted-foreground">Loading...</p>
        </div>
      }
    >
      <TasksPageContent />
    </Suspense>
  );
}
