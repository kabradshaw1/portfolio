interface RadialGaugeProps {
  value: number | null;
  label: string;
  size?: number;
}

function scoreColor(value: number): string {
  if (value >= 0.7) return "#22c55e"; // green
  if (value >= 0.4) return "#eab308"; // yellow
  return "#ef4444"; // red
}

export function RadialGauge({ value, label, size = 80 }: RadialGaugeProps) {
  const radius = size * 0.4;
  const circumference = 2 * Math.PI * radius;
  const center = size / 2;
  const strokeWidth = size * 0.08;

  const displayValue = value !== null ? value : 0;
  const offset = circumference - displayValue * circumference;
  const color = value !== null ? scoreColor(value) : "#64748b";

  return (
    <div className="flex flex-col items-center gap-1">
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        {/* Background track */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke="#1e293b"
          strokeWidth={strokeWidth}
        />
        {/* Score arc */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          transform={`rotate(-90 ${center} ${center})`}
        />
        {/* Score text */}
        <text
          x={center}
          y={center + size * 0.05}
          textAnchor="middle"
          fill="white"
          fontSize={size * 0.18}
          fontWeight="bold"
        >
          {value !== null ? value.toFixed(2) : "N/A"}
        </text>
      </svg>
      <span className="text-xs text-muted-foreground">{label}</span>
    </div>
  );
}
