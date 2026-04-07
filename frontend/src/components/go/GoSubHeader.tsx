"use client";

import { usePathname } from "next/navigation";
import { GoCartIcon } from "@/components/go/GoCartIcon";
import { GoUserDropdown } from "@/components/go/GoUserDropdown";
import { useGoStore } from "@/components/go/GoStoreProvider";

export function GoSubHeader() {
  const pathname = usePathname();
  const onStore = pathname === "/go/ecommerce";
  const { categories, activeCategory, setActiveCategory } = useGoStore();

  return (
    <div className="border-b border-foreground/10 bg-background">
      <div className="mx-auto flex h-12 max-w-5xl items-center gap-4 px-6">
        {onStore && <h1 className="text-lg font-semibold">Store</h1>}
        {onStore && (
          <div className="flex flex-1 flex-wrap items-center gap-2">
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
          </div>
        )}
        <div className="ml-auto flex items-center gap-4">
          <GoCartIcon />
          <GoUserDropdown />
        </div>
      </div>
    </div>
  );
}
