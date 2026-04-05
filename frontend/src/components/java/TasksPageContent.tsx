"use client";

import { useEffect, useState } from "react";
import { useAuth } from "@/components/java/AuthProvider";
import { GoogleLoginButton } from "@/components/java/GoogleLoginButton";
import { RegisterForm } from "@/components/java/RegisterForm";
import { ForgotPasswordForm } from "@/components/java/ForgotPasswordForm";
import { ProjectList } from "@/components/java/ProjectList";
import { useSearchParams } from "next/navigation";

type AuthView = "login" | "register" | "forgot-password";

export function TasksPageContent() {
  const { isLoggedIn, login, loginWithPassword } = useAuth();
  const searchParams = useSearchParams();
  const [view, setView] = useState<AuthView>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const resetSuccess = searchParams.get("reset") === "success";

  // Handle OAuth callback
  useEffect(() => {
    const code = searchParams.get("code");
    if (code && !isLoggedIn) {
      const redirectUri = `${window.location.origin}/java/tasks`;
      login(code, redirectUri).then(() => {
        window.history.replaceState({}, "", "/java/tasks");
      });
    }
  }, [searchParams, isLoggedIn, login]);

  if (isLoggedIn) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <ProjectList />
      </div>
    );
  }

  if (view === "register") {
    return (
      <div className="mx-auto max-w-sm px-6 py-24">
        <RegisterForm onBack={() => setView("login")} />
      </div>
    );
  }

  if (view === "forgot-password") {
    return (
      <div className="mx-auto max-w-sm px-6 py-24">
        <ForgotPasswordForm onBack={() => setView("login")} />
      </div>
    );
  }

  const handlePasswordLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await loginWithPassword(email, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="mx-auto max-w-sm px-6 py-24">
      <div className="flex flex-col gap-6">
        <div className="text-center">
          <h1 className="text-2xl font-semibold">Task Manager</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Sign in to manage your projects and tasks.
          </p>
        </div>

        {resetSuccess && (
          <p className="text-sm text-green-500 text-center">
            Password reset successful. You can now sign in.
          </p>
        )}

        {error && (
          <p className="text-sm text-red-500 text-center">{error}</p>
        )}

        <form onSubmit={handlePasswordLogin} className="flex flex-col gap-4">
          <input
            type="email"
            placeholder="Email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            className="rounded-md border border-foreground/20 bg-background px-3 py-2 text-sm"
          />
          <input
            type="password"
            placeholder="Password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            className="rounded-md border border-foreground/20 bg-background px-3 py-2 text-sm"
          />
          <button
            type="submit"
            disabled={loading}
            className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {loading ? "Signing in..." : "Sign in"}
          </button>
        </form>

        <div className="flex justify-between text-sm">
          <button
            onClick={() => setView("forgot-password")}
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            Forgot password?
          </button>
          <button
            onClick={() => setView("register")}
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            Create account
          </button>
        </div>

        <div className="relative">
          <div className="absolute inset-0 flex items-center">
            <div className="w-full border-t border-foreground/10" />
          </div>
          <div className="relative flex justify-center text-xs">
            <span className="bg-background px-2 text-muted-foreground">or</span>
          </div>
        </div>

        <div className="flex justify-center">
          <GoogleLoginButton />
        </div>
      </div>
    </div>
  );
}
