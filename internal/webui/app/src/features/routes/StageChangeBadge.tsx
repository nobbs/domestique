import type { StageChange } from "../../lib/seenStages";

/** New or changed since this reader last opened it. Text, never colour alone. */
export function StageChangeBadge({ change }: { change: StageChange }) {
  if (!change) {
    return null;
  }

  return (
    <span
      className="rounded-full bg-[var(--base)] px-1.5 py-0.5 text-[11px] font-semibold text-[var(--ink-2)]"
      data-change={change}
    >
      {change === "new" ? "New" : "Updated"}
    </span>
  );
}
