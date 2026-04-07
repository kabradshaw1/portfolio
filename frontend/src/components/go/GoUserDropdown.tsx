"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useGoAuth } from "@/components/go/GoAuthProvider";

function initials(name: string): string {
  return name
    .split(" ")
    .map((w) => w[0])
    .filter(Boolean)
    .slice(0, 2)
    .join("")
    .toUpperCase();
}

export function GoUserDropdown() {
  const router = useRouter();
  const { user, isLoggedIn, logout } = useGoAuth();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center outline-none" aria-label="Account menu">
        {isLoggedIn && user?.avatarUrl ? (
          <img
            src={user.avatarUrl}
            alt=""
            className="size-7 rounded-full"
          />
        ) : isLoggedIn && user ? (
          <span className="flex size-7 items-center justify-center rounded-full bg-muted text-xs font-semibold">
            {initials(user.name)}
          </span>
        ) : (
          <span className="text-sm text-muted-foreground hover:text-foreground transition-colors">
            Welcome
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {isLoggedIn && user ? (
          <>
            <DropdownMenuLabel className="font-normal">
              <div className="flex flex-col">
                <span className="text-sm font-medium">{user.name}</span>
                <span className="text-xs text-muted-foreground">{user.email}</span>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem render={<Link href="/go/ecommerce/orders" />}>
              Orders
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => {
                logout();
                router.push("/go/ecommerce");
              }}
            >
              Sign out
            </DropdownMenuItem>
          </>
        ) : (
          <>
            <DropdownMenuItem render={<Link href="/go/login" />}>
              Sign in
            </DropdownMenuItem>
            <DropdownMenuItem render={<Link href="/go/register" />}>
              Register
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
