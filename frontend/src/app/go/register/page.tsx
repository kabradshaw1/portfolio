"use client";

import { useCallback, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { GoGoogleLoginButton } from "@/components/go/GoGoogleLoginButton";
import { useGoAuth } from "@/components/go/GoAuthProvider";
import { HealthGate } from "@/components/HealthGate";

const goAuthUrl =
  process.env.NEXT_PUBLIC_GO_AUTH_URL || "http://localhost:8091";

export default function GoRegisterPage() {
  const router = useRouter();
  const { register } = useGoAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setError("");
      setBusy(true);
      try {
        await register(email, password, name);
        router.push("/go/ecommerce");
      } catch (err) {
        setError(err instanceof Error ? err.message : "Registration failed");
      } finally {
        setBusy(false);
      }
    },
    [email, password, name, register, router],
  );

  return (
    <HealthGate
      endpoint={`${goAuthUrl}/health`}
      stack="Go Ecommerce"
      docsHref="/go"
    >
      <div className="mx-auto max-w-sm px-6 py-12">
        <h1 className="text-2xl font-bold">Create account</h1>
        <form onSubmit={handleSubmit} className="mt-6 space-y-3">
          <input
            type="text"
            placeholder="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            disabled={busy}
            className="w-full rounded border border-foreground/20 bg-background px-3 py-2 text-sm"
          />
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
            placeholder="Password (min 12 chars)"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={12}
            disabled={busy}
            className="w-full rounded border border-foreground/20 bg-background px-3 py-2 text-sm"
          />
          <Button type="submit" disabled={busy} className="w-full">
            {busy ? "Creating account…" : "Create account"}
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
          Already have an account?{" "}
          <Link href="/go/login" className="underline hover:text-foreground">
            Sign in
          </Link>
        </p>
      </div>
    </HealthGate>
  );
}
