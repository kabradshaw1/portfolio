"use client";

import { GoSubHeader } from "@/components/go/GoSubHeader";
import { AiAssistantDrawer } from "@/components/go/AiAssistantDrawer";
import { HealthGate } from "@/components/HealthGate";

const goEcommerceUrl =
  process.env.NEXT_PUBLIC_GO_ECOMMERCE_URL || "http://localhost:8092";

export default function GoEcommerceLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <HealthGate
      endpoint={`${goEcommerceUrl}/health`}
      stack="Go Ecommerce"
      docsHref="/go"
    >
      <GoSubHeader />
      {children}
      <AiAssistantDrawer />
    </HealthGate>
  );
}
