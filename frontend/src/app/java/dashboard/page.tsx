"use client";

import { Suspense } from "react";
import { HealthGate } from "@/components/HealthGate";
import { DashboardClient } from "./dashboard-client";

const gatewayUrl =
  process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export default function DashboardPage() {
  return (
    <HealthGate
      endpoint={`${gatewayUrl}/actuator/health`}
      stack="Java Task Management"
      docsHref="/java"
    >
      <Suspense
        fallback={
          <div className="p-6 text-sm text-muted-foreground">Loading…</div>
        }
      >
        <DashboardClient />
      </Suspense>
    </HealthGate>
  );
}
