import { refreshGoAccessToken, GO_CART_URL } from "@/lib/go-auth";

export async function goCartFetch(
  path: string,
  options: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(options.headers);
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${GO_CART_URL}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });

  if (res.status === 401 || res.status === 403) {
    const success = await refreshGoAccessToken();
    if (success) {
      return fetch(`${GO_CART_URL}${path}`, {
        ...options,
        headers,
        credentials: "include",
      });
    }
  }

  return res;
}
