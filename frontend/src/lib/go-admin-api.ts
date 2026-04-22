const GO_ORDER_URL =
  process.env.NEXT_PUBLIC_GO_ORDER_URL || "http://localhost:8092";

export interface DLQMessage {
  index: number;
  routing_key: string;
  exchange: string;
  timestamp: string;
  retry_count: number;
  headers: Record<string, unknown>;
  body: unknown;
}

export interface DLQListResponse {
  messages: DLQMessage[];
  count: number;
}

export interface DLQReplayResponse {
  replayed: DLQMessage;
}

export interface DLQResult<T> {
  data: T | null;
  error: string | null;
  connected: boolean;
}

export async function fetchDLQMessages(
  limit = 50,
): Promise<DLQResult<DLQListResponse>> {
  try {
    const res = await fetch(
      `${GO_ORDER_URL}/admin/dlq/messages?limit=${limit}`,
    );
    if (!res.ok) {
      return { data: null, error: `HTTP ${res.status}`, connected: true };
    }
    const data: DLQListResponse = await res.json();
    return { data, error: null, connected: true };
  } catch {
    return { data: null, error: null, connected: false };
  }
}

export async function replayDLQMessage(
  index: number,
): Promise<DLQResult<DLQReplayResponse>> {
  try {
    const res = await fetch(`${GO_ORDER_URL}/admin/dlq/replay`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ index }),
    });
    if (!res.ok) {
      const text = await res.text();
      return { data: null, error: text || `HTTP ${res.status}`, connected: true };
    }
    const data: DLQReplayResponse = await res.json();
    return { data, error: null, connected: true };
  } catch {
    return { data: null, error: null, connected: false };
  }
}
