"use client";

import { useCallback } from "react";
import { Button } from "@/components/ui/button";
import { GOOGLE_CLIENT_ID } from "@/lib/auth";

export function GoogleLoginButton() {
  const handleLogin = useCallback(() => {
    const redirectUri = `${window.location.origin}/java/tasks`;
    const state = crypto.randomUUID();
    sessionStorage.setItem("java_oauth_state", state);
    const params = new URLSearchParams({
      client_id: GOOGLE_CLIENT_ID,
      redirect_uri: redirectUri,
      response_type: "code",
      scope: "openid email profile",
      access_type: "offline",
      prompt: "consent",
      state,
    });
    window.location.href = `https://accounts.google.com/o/oauth2/v2/auth?${params}`;
  }, []);

  return (
    <Button onClick={handleLogin} size="lg">
      Sign in with Google
    </Button>
  );
}
