import type { Metadata } from "next";
import type { ReactNode } from "react";

const DESCRIPTION =
  "Watch a four-agent LangGraph pipeline triage, investigate, validate, and " +
  "report on a security incident — with real per-role token, cost, and latency " +
  "numbers. Replays captured runs (no backend required) or streams a live one.";

export const metadata: Metadata = {
  title: "IR Agent — Live Demo — Kyle Bradshaw",
  description: DESCRIPTION,
  openGraph: {
    title: "IR Agent — Live Demo",
    description: DESCRIPTION,
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "IR Agent — Live Demo",
    description: DESCRIPTION,
  },
};

export default function IRAgentDemoLayout({
  children,
}: {
  children: ReactNode;
}) {
  return children;
}
