"use client";

import { useEffect, useRef } from "react";
import mermaid from "mermaid";
import DOMPurify from "dompurify";

mermaid.initialize({
  startOnLoad: false,
  theme: "dark",
  themeVariables: {
    darkMode: true,
  },
});

export function MermaidDiagram({ chart }: { chart: string }) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!ref.current) return;

    const id = `mermaid-${Date.now()}`;
    mermaid.render(id, chart).then(({ svg }) => {
      if (ref.current) {
        ref.current.innerHTML = DOMPurify.sanitize(svg);
      }
    });
  }, [chart]);

  return <div ref={ref} className="flex justify-center overflow-x-auto" />;
}
