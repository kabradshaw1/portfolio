"use client";

import { useEffect, useState } from "react";

export type StickyTocItem = {
  id: string;
  label: string;
};

export type StickyTocProps = {
  items: StickyTocItem[];
};

/** Tracks which section is currently most-prominent in the viewport. */
function useActiveSection(items: StickyTocItem[]): string {
  const [activeId, setActiveId] = useState<string>(items[0]?.id ?? "");

  useEffect(() => {
    if (typeof window === "undefined" || items.length === 0) return;

    const sections = items
      .map((item) => document.getElementById(item.id))
      .filter((el): el is HTMLElement => el !== null);

    if (sections.length === 0) return;

    // Pick the section closest to the top of the viewport that is still
    // intersecting. The rootMargin biases toward sections whose top has
    // crossed about 25% of the viewport, avoiding flicker when two sections
    // are partially in view.
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible.length > 0) {
          setActiveId(visible[0].target.id);
        }
      },
      {
        rootMargin: "-25% 0px -65% 0px",
        threshold: 0,
      },
    );

    sections.forEach((section) => observer.observe(section));
    return () => observer.disconnect();
  }, [items]);

  return activeId;
}

/** Mobile horizontal chip row — render once at the top of the tab content. */
export function StickyTocChips({ items }: StickyTocProps) {
  const activeId = useActiveSection(items);
  return (
    <nav
      aria-label="Section navigation"
      className="-mx-6 mb-6 overflow-x-auto px-6"
      data-testid="sticky-toc-chips"
    >
      <ul className="flex gap-2 whitespace-nowrap">
        {items.map((item) => (
          <li key={item.id}>
            <a
              href={`#${item.id}`}
              className={`inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium transition-colors ${
                activeId === item.id
                  ? "border-primary text-foreground bg-accent"
                  : "border-foreground/10 text-muted-foreground hover:text-foreground"
              }`}
            >
              {item.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

/** Desktop sticky right-column TOC — render once inside an `<aside>`. */
export function StickyTocSidebar({ items }: StickyTocProps) {
  const activeId = useActiveSection(items);
  return (
    <nav
      aria-label="Section navigation"
      className="sticky top-24 self-start"
      data-testid="sticky-toc-sidebar"
    >
      <p className="text-xs uppercase tracking-wide text-muted-foreground mb-3">On this page</p>
      <ul className="space-y-2 border-l border-foreground/10">
        {items.map((item) => (
          <li key={item.id}>
            <a
              href={`#${item.id}`}
              className={`block border-l-2 pl-3 -ml-px text-sm transition-colors ${
                activeId === item.id
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              {item.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
