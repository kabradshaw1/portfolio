import { ToolCallCard } from "./ToolCallCard";
import { DiagnosisBanner } from "./DiagnosisBanner";

export interface AgentEvent {
  event: "thinking" | "tool_call" | "tool_result" | "diagnosis" | "done";
  data: {
    content?: string;
    tool?: string;
    args?: Record<string, unknown>;
    result?: unknown;
    truncated?: boolean;
    step?: number;
  };
}

interface AgentTimelineProps {
  events: AgentEvent[];
}

export function AgentTimeline({ events }: AgentTimelineProps) {
  if (events.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-center text-muted-foreground">
        <p className="text-sm">
          Agent output will appear here as the debug session runs.
        </p>
      </div>
    );
  }

  // Pair tool_call events with their matching tool_result by step number
  const rendered: React.ReactNode[] = [];
  const usedResultSteps = new Set<number>();

  events.forEach((event, i) => {
    if (event.event === "thinking") {
      rendered.push(
        <p key={i} className="text-sm italic text-muted-foreground">
          {event.data.content ?? ""}
        </p>
      );
    } else if (event.event === "tool_call") {
      const step = event.data.step ?? i;
      // Find matching tool_result
      const resultEvent = events.find(
        (e) => e.event === "tool_result" && e.data.step === step
      );
      if (resultEvent) {
        usedResultSteps.add(step);
      }
      rendered.push(
        <ToolCallCard
          key={i}
          step={step}
          tool={event.data.tool ?? "unknown"}
          args={event.data.args ?? {}}
          result={resultEvent?.data.result}
          truncated={resultEvent?.data.truncated}
        />
      );
    } else if (event.event === "tool_result") {
      const step = event.data.step ?? i;
      // Only render standalone if not already paired with a tool_call
      if (!usedResultSteps.has(step)) {
        rendered.push(
          <ToolCallCard
            key={i}
            step={step}
            tool="result"
            args={{}}
            result={event.data.result}
            truncated={event.data.truncated}
          />
        );
      }
    } else if (event.event === "diagnosis") {
      rendered.push(
        <DiagnosisBanner key={i} content={event.data.content ?? ""} />
      );
    }
    // "done" events produce no visible output
  });

  return <div className="flex flex-col gap-4">{rendered}</div>;
}
