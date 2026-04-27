import { IncidentCard, type Incident } from "./IncidentCard";

export function JourneyTimeline({ incidents }: { incidents: Incident[] }) {
  return (
    <ol className="relative space-y-6 sm:space-y-8">
      {incidents.map((incident, idx) => (
        <li
          key={incident.adrId}
          className="grid grid-cols-1 gap-3 sm:grid-cols-[120px_1fr] sm:gap-6"
        >
          <div className="flex sm:flex-col sm:items-end sm:pt-5">
            <span className="font-mono text-xs uppercase tracking-wide text-muted-foreground">
              {incident.date}
            </span>
            <span className="ml-2 sm:ml-0 sm:mt-1 font-mono text-xs text-muted-foreground/70">
              #{idx + 1}
            </span>
          </div>
          <IncidentCard incident={incident} />
        </li>
      ))}
    </ol>
  );
}
