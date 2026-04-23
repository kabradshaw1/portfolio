"use client";

import { AiAssistantDrawer } from "@/components/go/AiAssistantDrawer";
import { HealthGate } from "@/components/HealthGate";

const goOrderUrl =
  process.env.NEXT_PUBLIC_GO_ORDER_URL || "http://localhost:8092";

export default function GoEcommerceLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <HealthGate
      endpoint={`${goOrderUrl}/health`}
      stack="Go Ecommerce"
      docsHref="/go"
      degraded
    >
      {children}
      <AiAssistantDrawer />
    </HealthGate>
  );
}
