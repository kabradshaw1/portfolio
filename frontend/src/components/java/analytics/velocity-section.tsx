"use client";

import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatCard } from "./stat-card";
import type { VelocityMetrics } from "./project-health.graphql";

interface Props { velocity: VelocityMetrics; }

export function VelocitySection({ velocity }: Props) {
  const data = [...velocity.weeklyThroughput].reverse();
  const empty = data.length === 0;

  const config = {
    completed: { label: "Completed", color: "hsl(var(--chart-1))" },
    created: { label: "Created", color: "hsl(var(--chart-2))" },
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader><CardTitle>Weekly Throughput</CardTitle></CardHeader>
        <CardContent>
          {empty ? (
            <p className="text-sm text-muted-foreground">No data yet</p>
          ) : (
            <ChartContainer config={config} className="h-56 w-full">
              <AreaChart data={data}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="week" tickLine={false} axisLine={false} />
                <YAxis allowDecimals={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Area type="monotone" dataKey="completed" stroke="var(--color-completed)" fill="var(--color-completed)" fillOpacity={0.3} />
                <Area type="monotone" dataKey="created" stroke="var(--color-created)" fill="var(--color-created)" fillOpacity={0.2} />
              </AreaChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
      <div className="grid gap-3 sm:grid-cols-4">
        <StatCard label="Avg Lead Time" value={velocity.avgLeadTimeHours} unit="h" />
        <StatCard label="p50" value={velocity.leadTimePercentiles.p50} unit="h" />
        <StatCard label="p75" value={velocity.leadTimePercentiles.p75} unit="h" />
        <StatCard label="p95" value={velocity.leadTimePercentiles.p95} unit="h" />
      </div>
    </div>
  );
}
