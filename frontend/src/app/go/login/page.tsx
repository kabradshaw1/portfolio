"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Button } from "@/components/ui/button";
import { GoGoogleLoginButton } from "@/components/go/GoGoogleLoginButton";
import { useGoAuth } from "@/components/go/GoAuthProvider";
import { HealthGate } from "@/components/HealthGate";

const goAuthUrl =
  process.env.NEXT_PUBLIC_GO_AUTH_URL || "http://localhost:8091";

export default function GoLoginPage() {
  return (
    <HealthGate
      endpoint={`${goAuthUrl}/health`}
      stack="Go Ecommerce"
      docsHref="/go"
    >
      <Suspense fallback={<div className="mx-auto max-w-sm px-6 py-12">Loading…</div>}>
        <GoLoginPageInner />
      </Suspense>
    </HealthGate>
  );
}

function GoLoginPageInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { login, loginWithGoogle } = useGoAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const code = searchParams.get("code");
    if (!code) return;
    let cancelled = false;
    (async () => {
      setBusy(true);
      try {
        const redirectUri = `${window.location.origin}/go/login`;
        await loginWithGoogle(code, redirectUri);
        const stored = sessionStorage.getItem("go_login_next");
        sessionStorage.removeItem("go_login_next");
        if (!cancelled) router.replace(stored || searchParams.get("next") || "/go/ecommerce");
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Google sign-in failed");
          router.replace("/go/login");
        }
      } finally {
        if (!cancelled) setBusy(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [searchParams, loginWithGoogle, router]);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setError("");
      setBusy(true);
      try {
        await login(email, password);
        router.push(searchParams.get("next") || "/go/ecommerce");
      } catch (err) {
        setError(err instanceof Error ? err.message : "Login failed");
      } finally {
        setBusy(false);
      }
    },
    [email, password, login, router],
  );

  return (
    <div className="mx-auto max-w-sm px-6 py-12">
      <h1 className="text-2xl font-bold">Sign in</h1>
      <form onSubmit={handleSubmit} className="mt-6 space-y-3">
        <input
          type="email"
          placeholder="Email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
          disabled={busy}
          className="w-full rounded border border-foreground/20 bg-background px-3 py-2 text-sm"
        />
        <input
          type="password"
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
          disabled={busy}
          className="w-full rounded border border-foreground/20 bg-background px-3 py-2 text-sm"
        />
        <Button type="submit" disabled={busy} className="w-full">
          {busy ? "Signing in…" : "Sign in"}
        </Button>
      </form>

      <div className="my-6 flex items-center gap-3 text-xs text-muted-foreground">
        <div className="h-px flex-1 bg-foreground/10" />
        <span>or</span>
        <div className="h-px flex-1 bg-foreground/10" />
      </div>

      <GoGoogleLoginButton />

      {error && <p className="mt-4 text-sm text-red-500">{error}</p>}

      <p className="mt-6 text-center text-sm text-muted-foreground">
        No account?{" "}
        <Link href="/go/register" className="underline hover:text-foreground">
          Register
        </Link>
      </p>
    </div>
  );
}
