"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useGoAuth } from "@/components/go/GoAuthProvider";

export function GoUserDropdown() {
  const router = useRouter();
  const { user, isLoggedIn, logout } = useGoAuth();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="text-sm text-muted-foreground hover:text-foreground transition-colors outline-none"
        aria-label="Account menu"
      >
        {isLoggedIn && user ? user.email : "Welcome"}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {isLoggedIn && user ? (
          <>
            <DropdownMenuGroup>
              <DropdownMenuLabel className="font-normal">
                <div className="flex flex-col">
                  <span className="text-sm font-medium">{user.name}</span>
                  <span className="text-xs text-muted-foreground">{user.email}</span>
                </div>
              </DropdownMenuLabel>
            </DropdownMenuGroup>
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
