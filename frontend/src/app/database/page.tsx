"use client";

import { useState } from "react";
import { databaseTabs, type DatabaseTab } from "@/components/database/tabs";
import { PostgresTab } from "@/components/database/PostgresTab";
import { RedisTab } from "@/components/database/RedisTab";
import { MongoDbTab } from "@/components/database/MongoDbTab";
import { VectorTab } from "@/components/database/VectorTab";

export default function DatabasePage() {
  const [activeTab, setActiveTab] = useState<DatabaseTab>("postgres");

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Database Engineering</h1>

      {/* Bio Section */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Production-grade PostgreSQL: real-database benchmarks (with measured
          3.5× wins), slow-query observability via{" "}
          <code>pg_stat_statements</code> + <code>auto_explain</code>,
          point-in-time recovery with verified backups, a custom AST-based
          migration linter, and range partitioning with materialized views for
          reporting. Redis underpins caching, rate limiting, HTTP idempotency,
          JWT revocation, and time-windowed analytics across the Go stack and
          Spring <code>@Cacheable</code> in the Java stack. MongoDB and Qdrant
          are also in use elsewhere — dedicated tabs for each are coming.
        </p>
      </section>

      {/* Project Section with Tabs */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Database Tracks</h2>

        {/* Tab Bar */}
        <div className="mt-4 flex gap-0 border-b border-foreground/10">
          {databaseTabs.map((tab) => (
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
        <div className="mt-8">
          {activeTab === "postgres" && <PostgresTab />}
          {activeTab === "redis" && <RedisTab />}
          {activeTab === "mongodb" && <MongoDbTab />}
          {activeTab === "vector" && <VectorTab />}
        </div>
      </section>
    </div>
  );
}
