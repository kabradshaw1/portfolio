"use client";

import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { ProjectStats } from "./project-health.graphql";

interface Props { stats: ProjectStats; }

export function TaskBreakdownCharts({ stats }: Props) {
  const statusData = [
    { name: "TODO", count: stats.taskCountByStatus.todo },
    { name: "IN PROGRESS", count: stats.taskCountByStatus.inProgress },
    { name: "DONE", count: stats.taskCountByStatus.done },
  ];
  const priorityData = [
    { name: "LOW", count: stats.taskCountByPriority.low },
    { name: "MEDIUM", count: stats.taskCountByPriority.medium },
    { name: "HIGH", count: stats.taskCountByPriority.high },
  ];
  const statusEmpty = statusData.every((d) => d.count === 0);
  const priorityEmpty = priorityData.every((d) => d.count === 0);

  const config = { count: { label: "Tasks", color: "hsl(var(--chart-1))" } };

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader><CardTitle>By Status</CardTitle></CardHeader>
        <CardContent>
          {statusEmpty ? (
            <p className="text-sm text-muted-foreground">No data yet</p>
          ) : (
            <ChartContainer config={config} className="h-48 w-full">
              <BarChart data={statusData}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="name" tickLine={false} axisLine={false} />
                <YAxis allowDecimals={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Bar dataKey="count" fill="var(--color-count)" radius={4} />
              </BarChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>By Priority</CardTitle></CardHeader>
        <CardContent>
          {priorityEmpty ? (
            <p className="text-sm text-muted-foreground">No data yet</p>
          ) : (
            <ChartContainer config={config} className="h-48 w-full">
              <BarChart data={priorityData}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="name" tickLine={false} axisLine={false} />
                <YAxis allowDecimals={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Bar dataKey="count" fill="var(--color-count)" radius={4} />
              </BarChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
