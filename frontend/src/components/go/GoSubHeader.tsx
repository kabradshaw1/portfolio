"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { GoCartIcon } from "@/components/go/GoCartIcon";
import { GoUserDropdown } from "@/components/go/GoUserDropdown";
import { useGoStore } from "@/components/go/GoStoreProvider";

export function GoSubHeader() {
  const pathname = usePathname();
  const inStore = pathname.startsWith("/go/ecommerce");
  const onStoreRoot = pathname === "/go/ecommerce";
  const { categories, activeCategory, setActiveCategory } = useGoStore();

  return (
    <div className="border-b border-foreground/10 bg-background">
      <div className="mx-auto grid h-12 max-w-5xl grid-cols-[1fr_auto_1fr] items-center gap-4 px-6">
        <div className="flex items-center">
          {inStore && (
            <Link href="/go/ecommerce" className="text-lg font-semibold hover:text-primary transition-colors">
              Store
            </Link>
          )}
        </div>
        <div className="flex flex-wrap items-center justify-center gap-2">
          {onStoreRoot && (
            <>
              <button
                onClick={() => setActiveCategory(null)}
                className={`rounded-full px-3 py-1 text-sm transition-colors ${
                  activeCategory === null
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground hover:text-foreground"
                }`}
              >
                All
              </button>
              {categories.map((cat) => (
                <button
                  key={cat}
                  onClick={() => setActiveCategory(cat)}
                  className={`rounded-full px-3 py-1 text-sm transition-colors ${
                    activeCategory === cat
                      ? "bg-primary text-primary-foreground"
                      : "bg-muted text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {cat}
                </button>
              ))}
            </>
          )}
        </div>
        <div className="flex items-center justify-end gap-4">
          <GoCartIcon />
          <GoUserDropdown />
        </div>
      </div>
    </div>
  );
}
