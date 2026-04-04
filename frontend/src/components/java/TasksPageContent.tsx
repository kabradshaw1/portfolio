"use client";

import { useEffect } from "react";
import { useAuth } from "@/components/java/AuthProvider";
import { GoogleLoginButton } from "@/components/java/GoogleLoginButton";
import { ProjectList } from "@/components/java/ProjectList";
import { useSearchParams } from "next/navigation";

export function TasksPageContent() {
  const { isLoggedIn, login } = useAuth();
  const searchParams = useSearchParams();

  // Handle OAuth callback
  useEffect(() => {
    const code = searchParams.get("code");
    if (code && !isLoggedIn) {
      const redirectUri = `${window.location.origin}/java/tasks`;
      login(code, redirectUri).then(() => {
        // Remove code from URL
        window.history.replaceState({}, "", "/java/tasks");
      });
    }
  }, [searchParams, isLoggedIn, login]);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      {!isLoggedIn ? (
        <div className="flex flex-col items-center gap-6 py-24">
          <h1 className="text-2xl font-semibold">Task Manager</h1>
          <p className="text-muted-foreground">
            Sign in to manage your projects and tasks.
          </p>
          <GoogleLoginButton />
        </div>
      ) : (
        <ProjectList />
      )}
    </div>
  );
}
