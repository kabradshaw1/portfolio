export const GO_AUTH_URL =
  process.env.NEXT_PUBLIC_GO_AUTH_URL || "http://localhost:8091";
export const GO_ECOMMERCE_URL =
  process.env.NEXT_PUBLIC_GO_ECOMMERCE_URL || "http://localhost:8092";

export async function refreshGoAccessToken(): Promise<boolean> {
  try {
    const res = await fetch(`${GO_AUTH_URL}/auth/refresh`, {
      method: "POST",
      credentials: "include",
    });
    if (!res.ok) {
      if (typeof window !== "undefined") {
        window.dispatchEvent(new Event("go-auth-cleared"));
      }
      return false;
    }
    return true;
  } catch {
    if (typeof window !== "undefined") {
      window.dispatchEvent(new Event("go-auth-cleared"));
    }
    return false;
  }
}
