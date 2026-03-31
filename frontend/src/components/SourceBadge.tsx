import { Badge } from "@/components/ui/badge";

interface SourceBadgeProps {
  filename: string;
  page: number;
}

export function SourceBadge({ filename, page }: SourceBadgeProps) {
  return (
    <Badge variant="secondary" className="text-xs font-normal">
      {filename}, p.{page}
    </Badge>
  );
}
