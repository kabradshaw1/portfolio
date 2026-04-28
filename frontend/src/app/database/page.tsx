"use client";

import { useState } from "react";
import { databaseTabs, type DatabaseTab } from "@/components/database/tabs";
import { PostgresTab } from "@/components/database/PostgresTab";
import { NoSqlTab } from "@/components/database/NoSqlTab";
import { VectorTab } from "@/components/database/VectorTab";

export default function DatabasePage() {
  const [activeTab, setActiveTab] = useState<DatabaseTab>("postgres");

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Database Engineering</h1>

      {/* Bio Section */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Production-grade PostgreSQL is one of the load-bearing skills behind
          this portfolio: real-database benchmarks, range partitioning with
          materialized views, a custom AST-based migration linter, and an
          operational track with backups and recovery runbooks. MongoDB and
          Qdrant are also in use elsewhere in the portfolio — dedicated tabs
          for each are coming.
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
          {activeTab === "nosql" && <NoSqlTab />}
          {activeTab === "vector" && <VectorTab />}
        </div>
      </section>
    </div>
  );
}
