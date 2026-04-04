"use client";

import { useQuery, useMutation } from "@apollo/client/react";
import { gql } from "@apollo/client";
import { Bell } from "lucide-react";
import { useState, useRef, useEffect } from "react";

const GET_NOTIFICATIONS = gql`
  query MyNotifications($unreadOnly: Boolean) {
    myNotifications(unreadOnly: $unreadOnly) {
      notifications {
        id
        type
        message
        taskId
        read
        createdAt
      }
      unreadCount
    }
  }
`;

const MARK_READ = gql`
  mutation MarkNotificationRead($id: ID!) {
    markNotificationRead(id: $id)
  }
`;

const MARK_ALL_READ = gql`
  mutation MarkAllNotificationsRead {
    markAllNotificationsRead
  }
`;

export function NotificationBell() {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const { data, refetch } = useQuery(GET_NOTIFICATIONS, {
    variables: { unreadOnly: false },
    pollInterval: 30000,
  });
  const [markRead] = useMutation(MARK_READ);
  const [markAllRead] = useMutation(MARK_ALL_READ);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const unreadCount = (
    data as { myNotifications?: { unreadCount?: number } } | undefined
  )?.myNotifications?.unreadCount ?? 0;
  const notifications = (
    data as {
      myNotifications?: {
        notifications?: Array<{
          id: string;
          message: string;
          read: boolean;
          createdAt: string;
        }>;
      };
    } | undefined
  )?.myNotifications?.notifications ?? [];

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        className="relative text-muted-foreground hover:text-foreground transition-colors"
      >
        <Bell className="size-5" />
        {unreadCount > 0 && (
          <span className="absolute -top-1 -right-1 flex size-4 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-primary-foreground">
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-8 z-50 w-80 rounded-xl border border-foreground/10 bg-background shadow-lg">
          <div className="flex items-center justify-between border-b border-foreground/10 px-4 py-2">
            <span className="text-sm font-medium">Notifications</span>
            {unreadCount > 0 && (
              <button
                onClick={async () => {
                  await markAllRead();
                  refetch();
                }}
                className="text-xs text-primary hover:underline"
              >
                Mark all read
              </button>
            )}
          </div>
          <div className="max-h-64 overflow-y-auto">
            {notifications.length === 0 ? (
              <p className="px-4 py-6 text-sm text-muted-foreground text-center">
                No notifications
              </p>
            ) : (
              notifications.map(
                (n: {
                  id: string;
                  message: string;
                  read: boolean;
                  createdAt: string;
                }) => (
                  <div
                    key={n.id}
                    className={`px-4 py-3 border-b border-foreground/5 cursor-pointer hover:bg-muted/50 ${
                      !n.read ? "bg-primary/5" : ""
                    }`}
                    onClick={async () => {
                      if (!n.read) {
                        await markRead({ variables: { id: n.id } });
                        refetch();
                      }
                    }}
                  >
                    <p className="text-sm">{n.message}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {new Date(n.createdAt).toLocaleString()}
                    </p>
                  </div>
                )
              )
            )}
          </div>
        </div>
      )}
    </div>
  );
}
