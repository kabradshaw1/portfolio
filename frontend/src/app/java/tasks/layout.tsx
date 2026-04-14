"use client";

import { JavaSubHeader } from "@/components/java/JavaSubHeader";
import { HealthGate } from "@/components/HealthGate";

const gatewayUrl =
  process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export default function JavaTasksLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <HealthGate
      endpoint={`${gatewayUrl}/actuator/health`}
      stack="Java Task Management"
      docsHref="/java"
    >
      <JavaSubHeader />
      {children}
    </HealthGate>
  );
}
