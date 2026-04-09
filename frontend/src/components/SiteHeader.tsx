"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { FileText } from "lucide-react";

export function SiteHeader() {
  const pathname = usePathname();

  const isActive = (prefix: string) =>
    pathname === prefix || pathname.startsWith(prefix + "/");

  const navLinkClass = (prefix: string) =>
    isActive(prefix)
      ? "text-sm text-foreground border-b-2 border-foreground pb-px transition-colors"
      : "text-sm text-muted-foreground hover:text-foreground transition-colors";

  return (
    <header className="border-b border-foreground/10 bg-background">
      <div className="mx-auto flex h-14 max-w-5xl items-center justify-between px-6">
        <div className="flex items-center gap-6">
          <Link href="/" className="text-lg font-semibold">
            Kyle Bradshaw
          </Link>
          <nav className="flex items-center gap-4">
            <Link href="/ai" className={navLinkClass("/ai")}>
              AI
            </Link>
            <Link href="/java" className={navLinkClass("/java")}>
              Java
            </Link>
            <Link href="/go" className={navLinkClass("/go")}>
              Go
            </Link>
          </nav>
        </div>
        <div className="flex items-center gap-5">
          <a
            href="https://api.kylebradshaw.dev/grafana/"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Grafana
          </a>
          <a
            href="/resume.pdf"
            aria-label="Resume"
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            <FileText className="size-5" />
          </a>
        </div>
      </div>
    </header>
  );
}
