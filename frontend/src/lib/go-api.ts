import {
  getGoAccessToken,
  refreshGoAccessToken,
  GO_ECOMMERCE_URL,
} from "@/lib/go-auth";

export async function goApiFetch(
  path: string,
  options: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(options.headers);
  const token = getGoAccessToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${GO_ECOMMERCE_URL}${path}`, {
    ...options,
    headers,
  });

  // Retry once on 403 with a refreshed token
  if (res.status === 403) {
    const newToken = await refreshGoAccessToken();
    if (newToken) {
      headers.set("Authorization", `Bearer ${newToken}`);
      return fetch(`${GO_ECOMMERCE_URL}${path}`, {
        ...options,
        headers,
      });
    }
  }

  return res;
}
