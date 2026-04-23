"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { MicroservicesTab } from "@/components/go/tabs/MicroservicesTab";
import { OriginalTab } from "@/components/go/tabs/OriginalTab";
import { AiAssistantTab } from "@/components/go/tabs/AiAssistantTab";
import { AnalyticsTab } from "@/components/go/tabs/AnalyticsTab";
import { AdminTab } from "@/components/go/tabs/AdminTab";

type Tab = "microservices" | "original" | "ai-assistant" | "analytics" | "admin";

const tabs: { key: Tab; label: string }[] = [
  { key: "microservices", label: "Microservices" },
  { key: "original", label: "Original" },
  { key: "ai-assistant", label: "AI Assistant" },
  { key: "analytics", label: "Analytics" },
  { key: "admin", label: "Admin" },
];

export default function GoPage() {
  const [activeTab, setActiveTab] = useState<Tab>("microservices");

  useEffect(() => {
    function handleTabSwitch(e: Event) {
      const tab = (e as CustomEvent).detail as Tab;
      if (tabs.some((t) => t.key === tab)) {
        setActiveTab(tab);
      }
    }
    window.addEventListener("go-tab-switch", handleTabSwitch);
    return () => window.removeEventListener("go-tab-switch", handleTabSwitch);
  }, []);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Go Backend Developer</h1>

      {/* Bio Section */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Go is my preferred language due to its readability, simplicity, and
          strong performance. It&apos;s my first choice for many backend tasks,
          and I&apos;ve used it to build microservices, automation scripts, and
          command-line tools with a focus on clean, efficient design.
        </p>
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          All seven Go services expose Prometheus metrics to a live{" "}
          <a
            href="https://grafana.kylebradshaw.dev/d/system-overview/system-overview?orgId=1&from=now-1h&to=now&timezone=browser"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            Grafana dashboard
          </a>
          .
        </p>
      </section>

      {/* Project Section with Tabs */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Ecommerce Platform</h2>

        {/* Tab Bar */}
        <div className="mt-4 flex gap-0 border-b border-foreground/10">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
                activeTab === tab.key
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab Content */}
        {activeTab === "microservices" && <MicroservicesTab />}
        {activeTab === "original" && <OriginalTab />}
        {activeTab === "ai-assistant" && <AiAssistantTab />}
        {activeTab === "analytics" && <AnalyticsTab />}
        {activeTab === "admin" && <AdminTab />}

        {/* CTA Buttons */}
        <div className="mt-8 flex gap-3">
          <Link
            href="/go/ecommerce"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            View Store &rarr;
          </Link>
          <Link
            href="/go/analytics"
            className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
          >
            Streaming Analytics &rarr;
          </Link>
          <Link
            href="/go/admin"
            className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
          >
            Admin Panel &rarr;
          </Link>
        </div>
      </section>
    </div>
  );
}
