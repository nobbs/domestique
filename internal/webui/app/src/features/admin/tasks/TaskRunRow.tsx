/** One recorded task attempt: when it finished, what started it, and how it ended. */

import type { TaskRun } from "../../../api/types";
import { InsetRow } from "../../../components/InsetList";
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

/**
 * Only what got somewhere is painted as success: a gate that held, a shutdown,
 * and an outcome this build has never heard of are none of them a fault either.
 */
function tone(outcome: string): Tone {
  if (outcome === "failed") {
    return "alert";
  }
  if (outcome === "succeeded" || outcome === "unchanged") {
    return "good";
  }

  return "hold";
}

export function TaskRunRow({ run }: { run: TaskRun }) {
  const detail = taskDetailLabel(run.detail);

  return (
    <InsetRow
      data-outcome={run.outcome}
      tone={tone(run.outcome)}
      toneLabel={outcomeLabel(run.outcome)}
      title={`${run.task}${run.argument ? ` · ${run.argument}` : ""}`}
      detail={[formatTimestamp(run.finishedAt), triggerLabel(run.trigger), detail]}
      actions={
        run.reference === "" ? null : (
          <span className="text-xs text-[var(--ink-2)]">
            <span className="sr-only">Run reference </span>
            {run.reference}
          </span>
        )
      }
    />
  );
}
