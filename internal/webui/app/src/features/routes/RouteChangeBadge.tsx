import { Badge } from "../../components/ui/badge";
import type { RouteChange } from "../../lib/seenRoutes";

/** New or changed since this reader last opened it. Text, never colour alone. */
export function RouteChangeBadge({ change }: { change: RouteChange }) {
  if (!change) {
    return null;
  }

  return (
    <Badge
      variant="secondary"
      className="h-auto rounded-full bg-[var(--base)] px-1.5 py-0.5 text-[11px] font-semibold text-[var(--ink-2)]"
      data-change={change}
    >
      {change === "new" ? "New" : "Updated"}
    </Badge>
  );
}
