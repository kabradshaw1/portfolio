"use client";

import { useQuery } from "@apollo/client/react";
import { gql } from "@apollo/client";

const GET_ACTIVITY = gql`
  query TaskActivity($taskId: ID!) {
    taskActivity(taskId: $taskId) {
      id
      eventType
      actorId
      timestamp
    }
  }
`;

const eventLabels: Record<string, string> = {
  TASK_CREATED: "created this task",
  TASK_ASSIGNED: "assigned this task",
  STATUS_CHANGED: "changed the status",
  COMMENT_ADDED: "added a comment",
  TASK_DELETED: "deleted this task",
};

export function ActivityTimeline({ taskId }: { taskId: string }) {
  const { data, loading } = useQuery(GET_ACTIVITY, {
    variables: { taskId },
  });

  const events = (data as { taskActivity?: { id: string; eventType: string; actorId: string; timestamp: string }[] } | undefined)?.taskActivity ?? [];

  return (
    <div>
      <h3 className="text-lg font-semibold">Activity</h3>
      <div className="mt-4 space-y-2">
        {loading && <p className="text-sm text-muted-foreground">Loading...</p>}
        {events.map(
          (e: {
            id: string;
            eventType: string;
            actorId: string;
            timestamp: string;
          }) => (
            <div key={e.id} className="flex items-center gap-2 text-sm">
              <div className="size-2 rounded-full bg-muted-foreground" />
              <span className="text-muted-foreground">
                {eventLabels[e.eventType] || e.eventType}
              </span>
              <span className="text-xs text-muted-foreground">
                {new Date(e.timestamp).toLocaleString()}
              </span>
            </div>
          )
        )}
        {!loading && events.length === 0 && (
          <p className="text-sm text-muted-foreground">No activity yet.</p>
        )}
      </div>
    </div>
  );
}
