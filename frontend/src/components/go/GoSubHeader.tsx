"use client";

import { GoCartIcon } from "@/components/go/GoCartIcon";
import { GoUserDropdown } from "@/components/go/GoUserDropdown";

export function GoSubHeader() {
  return (
    <div className="border-b border-foreground/10 bg-background">
      <div className="mx-auto flex h-12 max-w-5xl items-center justify-end gap-4 px-6">
        <GoCartIcon />
        <GoUserDropdown />
      </div>
    </div>
  );
}
