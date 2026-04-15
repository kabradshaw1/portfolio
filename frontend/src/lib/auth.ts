export const GOOGLE_CLIENT_ID = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID || "";
export const GATEWAY_URL =
  process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export async function refreshAccessToken(): Promise<boolean> {
  try {
    const res = await fetch(`${GATEWAY_URL}/auth/refresh`, {
      method: "POST",
      credentials: "include",
    });
    if (!res.ok) {
      if (typeof window !== "undefined") {
        window.dispatchEvent(new Event("java-auth-cleared"));
      }
      return false;
    }
    return true;
  } catch {
    if (typeof window !== "undefined") {
      window.dispatchEvent(new Event("java-auth-cleared"));
    }
    return false;
  }
}
