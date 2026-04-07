"use client";

import { useCallback } from "react";
import { Button } from "@/components/ui/button";
import { GOOGLE_CLIENT_ID } from "@/lib/auth";

export function GoGoogleLoginButton() {
  const handleLogin = useCallback(() => {
    const next = new URLSearchParams(window.location.search).get("next");
    if (next) sessionStorage.setItem("go_login_next", next);
    const redirectUri = `${window.location.origin}/go/login`;
    const params = new URLSearchParams({
      client_id: GOOGLE_CLIENT_ID,
      redirect_uri: redirectUri,
      response_type: "code",
      scope: "openid email profile",
      access_type: "offline",
      prompt: "consent",
    });
    window.location.href = `https://accounts.google.com/o/oauth2/v2/auth?${params}`;
  }, []);

  return (
    <Button onClick={handleLogin} size="lg" variant="outline" className="w-full">
      Sign in with Google
    </Button>
  );
}
