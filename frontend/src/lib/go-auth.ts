const ACCESS_TOKEN_KEY = "go_access_token";
const REFRESH_TOKEN_KEY = "go_refresh_token";

export function getGoAccessToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getGoRefreshToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function setGoTokens(accessToken: string, refreshToken: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
}

export function clearGoTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

export function isGoLoggedIn(): boolean {
  return getGoAccessToken() !== null;
}

export const GO_AUTH_URL =
  process.env.NEXT_PUBLIC_GO_AUTH_URL || "http://localhost:8091";
export const GO_ECOMMERCE_URL =
  process.env.NEXT_PUBLIC_GO_ECOMMERCE_URL || "http://localhost:8092";

let refreshPromise: Promise<string | null> | null = null;

export async function refreshGoAccessToken(): Promise<string | null> {
  // Deduplicate concurrent refresh calls
  if (refreshPromise) return refreshPromise;

  refreshPromise = (async () => {
    const refreshToken = getGoRefreshToken();
    if (!refreshToken) return null;

    try {
      const res = await fetch(`${GO_AUTH_URL}/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refreshToken }),
      });
      if (!res.ok) {
        clearGoTokens();
        if (typeof window !== "undefined") {
          window.dispatchEvent(new Event("go-auth-cleared"));
        }
        return null;
      }
      const data = await res.json();
      setGoTokens(data.accessToken, data.refreshToken);
      return data.accessToken as string;
    } catch {
      clearGoTokens();
      if (typeof window !== "undefined") {
        window.dispatchEvent(new Event("go-auth-cleared"));
      }
      return null;
    } finally {
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}
