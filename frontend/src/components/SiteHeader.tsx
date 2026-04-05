"use client";

import Link from "next/link";
import { FileText } from "lucide-react";
import { useAuth } from "@/components/java/AuthProvider";
import { NotificationBell } from "@/components/java/NotificationBell";

export function SiteHeader() {
  const { user, isLoggedIn, logout } = useAuth();

  return (
    <header className="border-b border-foreground/10 bg-background">
      <div className="mx-auto flex h-14 max-w-5xl items-center justify-between px-6">
        <Link href="/" className="text-lg font-semibold">
          Kyle Bradshaw
        </Link>

        <nav className="flex items-center gap-4">
          <a
            href="https://github.com/kabradshaw1/portfolio"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Portfolio
          </a>
          <a
            href="https://github.com/kabradshaw1"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            GitHub
          </a>
          <a
            href="https://www.linkedin.com/in/kyle-bradshaw-15950988/"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            LinkedIn
          </a>
          <a
            href="/resume.pdf"
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            <FileText className="size-5" />
          </a>

          {isLoggedIn && (
            <>
              <NotificationBell />
              <div className="flex items-center gap-2">
                {user?.avatarUrl && (
                  <img
                    src={user.avatarUrl}
                    alt=""
                    className="size-7 rounded-full"
                  />
                )}
                <span className="text-sm text-muted-foreground">
                  {user?.name}
                </span>
                <button
                  onClick={logout}
                  className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                >
                  Sign out
                </button>
              </div>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}
