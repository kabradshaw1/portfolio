import { refreshGoAccessToken } from "@/lib/go-auth";

export const GO_PROJECTOR_URL =
  process.env.NEXT_PUBLIC_GO_PROJECTOR_URL || "http://localhost:8097";

export async function goProjectorFetch(
  path: string,
  options: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(options.headers);
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${GO_PROJECTOR_URL}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });

  // Retry once on 401/403 after refreshing the httpOnly cookie
  if (res.status === 401 || res.status === 403) {
    const success = await refreshGoAccessToken();
    if (success) {
      return fetch(`${GO_PROJECTOR_URL}${path}`, {
        ...options,
        headers,
        credentials: "include",
      });
    }
  }

  return res;
}

export interface TimelineEvent {
  eventId: string;
  orderId: string;
  eventType: string;
  eventVersion: number;
  data: Record<string, unknown>;
  timestamp: string;
}

export interface OrderSummary {
  orderId: string;
  userId: string;
  status: string;
  totalCents: number;
  currency: string;
  items?: Record<string, unknown>[];
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  failureReason?: string;
}

export interface OrderStats {
  hourBucket: string;
  ordersCreated: number;
  ordersCompleted: number;
  ordersFailed: number;
  avgCompletionSeconds: number;
  totalRevenueCents: number;
}
