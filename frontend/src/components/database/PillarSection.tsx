import type { ReactNode } from "react";

export type PillarLink = {
  label: string;
  href: string;
};

export type PillarSectionProps = {
  /** Anchor id, used by the sticky TOC and as the section's id. */
  id: string;
  /** Section heading, e.g. "Query Optimization & Benchmarking". */
  title: string;
  /** Two- to four-sentence narrative paragraph(s). */
  narrative: ReactNode;
  /** 4–6 concrete-claim bullets. Strings or ReactNodes. */
  bullets: ReactNode[];
  /** ADR / runbook links rendered as a muted button row. */
  links: PillarLink[];
};

export function PillarSection({ id, title, narrative, bullets, links }: PillarSectionProps) {
  return (
    <section id={id} className="scroll-mt-24" data-testid={`pillar-${id}`}>
      <h2 className="text-2xl font-semibold">{title}</h2>
      <div className="mt-4 text-muted-foreground leading-relaxed space-y-3">{narrative}</div>
      <ul className="mt-4 space-y-2 list-disc pl-5 text-sm text-muted-foreground">
        {bullets.map((bullet, i) => (
          <li key={i} className="leading-relaxed">
            {bullet}
          </li>
        ))}
      </ul>
      {links.length > 0 && (
        <div className="mt-5 flex flex-wrap gap-3">
          {links.map((link) => (
            <a
              key={link.href}
              href={link.href}
              className="inline-flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium hover:bg-accent transition-colors"
              target={link.href.startsWith("http") ? "_blank" : undefined}
              rel={link.href.startsWith("http") ? "noopener noreferrer" : undefined}
            >
              {link.label} &rarr;
            </a>
          ))}
        </div>
      )}
    </section>
  );
}
