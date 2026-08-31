/** One recorded task attempt: when it finished, what started it, and how it ended. */

import type { TaskRun } from "../../../api/types";
import { Badge } from "../../../components/ui/badge";
import { formatTimestamp } from "../../../lib/format";
import { outcomeLabel, taskDetailLabel } from "../../../lib/taskLabels";

const TRIGGER_LABELS: Record<string, string> = {
  schedule: "Scheduled",
  manual: "Manual",
  chain: "Chained",
};

/** What started the attempt, or "Unknown" for one recorded before this was written down. */
function triggerLabel(trigger: string | undefined): string {
  return trigger === undefined ? "Unknown" : (TRIGGER_LABELS[trigger] ?? trigger);
}

type Tone = "good" | "hold" | "alert";

/** A gate that held or a run still busy is neither a success nor a fault, and gets its own colour. */
function tone(outcome: string): Tone {
  if (outcome === "failed") {
    return "alert";
  }
  if (outcome === "blocked" || outcome === "not_ready" || outcome === "skipped") {
    return "hold";
  }

  return "good";
}

const TONE_CLASSES: Record<Tone, string> = {
  good: "border-[var(--good)] bg-[var(--base)] text-[var(--ink)]",
  hold: "border-[var(--hold)] bg-[var(--base)] text-[var(--ink)]",
  alert: "border-[var(--alert)] bg-[var(--base)] text-[var(--ink)]",
};

export function TaskRunRow({ run }: { run: TaskRun }) {
  const detail = taskDetailLabel(run.detail);

  return (
    <li
      className="grid gap-1 rounded-lg border border-[var(--rule)] p-3 text-sm sm:grid-cols-[1fr_auto]"
      data-outcome={run.outcome}
    >
      <span className="text-[var(--ink-2)]">{formatTimestamp(run.finishedAt)}</span>
      <span className="font-medium">
        {run.task}
        {run.argument ? ` · ${run.argument}` : ""}
      </span>
      <span className="text-[var(--ink-2)]">
        {triggerLabel(run.trigger)}
        {detail ? ` · ${detail}` : ""}
      </span>
      <span className="flex flex-wrap items-center gap-2 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:self-center">
        <Badge className={TONE_CLASSES[tone(run.outcome)]} variant="secondary">
          {outcomeLabel(run.outcome)}
        </Badge>
        {run.reference === "" ? null : (
          <span className="text-xs text-[var(--ink-2)]">
            <span className="sr-only">Run reference </span>
            {run.reference}
          </span>
        )}
      </span>
    </li>
  );
}
