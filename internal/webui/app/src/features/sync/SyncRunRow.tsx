import type { SyncRun } from "../../api/types";
import { Badge } from "../../components/ui/badge";
import { formatTimestamp } from "../../lib/format";
import { GUIDANCE_LABELS, syncGuidance } from "../../lib/syncGuidance";

/** How a recorded run ended, in one phrase. */
function runResult(run: SyncRun): string {
  const guidance = syncGuidance(run.phase, run.result, run.failure);

  return guidance ? GUIDANCE_LABELS[guidance.kind] : "Succeeded";
}

/**
 * What colour the outcome carries — the only colour on the row.
 *
 * A gate that held is not a failure and is not a success: it is a decision
 * waiting for the operator, so it gets its own word and its own colour rather
 * than borrowing the red of a run that broke.
 */
function runTone(run: SyncRun): "good" | "hold" | "alert" {
  const guidance = syncGuidance(run.phase, run.result, run.failure);
  if (!guidance) {
    return "good";
  }

  return guidance.kind === "blocked" ? "hold" : "alert";
}

/**
 * What a recorded run moved.
 *
 * A read is measured by what it found and a write by what it changed, which is
 * the same split the card above uses — the other half's counts are nought on
 * every row and would read as work that did not happen. A write that deleted
 * nothing says so by omission, because nought deleted is the ordinary case and
 * a zero on every row makes the one that is not zero harder to see.
 */
function runCounts(run: SyncRun): string {
  if (run.phase === "source") {
    return `${run.sourceRoutes} routes`;
  }

  return [
    `${run.created} created`,
    `${run.updated} updated`,
    ...(run.deleted > 0 ? [`${run.deleted} deleted`] : []),
  ].join(" · ");
}

const TONE_CLASSES: Record<ReturnType<typeof runTone>, string> = {
  good: "border-[var(--good)] bg-[var(--base)] text-[var(--ink)]",
  hold: "border-[var(--hold)] bg-[var(--base)] text-[var(--ink)]",
  alert: "border-[var(--alert)] bg-[var(--base)] text-[var(--ink)]",
};

/**
 * One recorded run: when it finished, which half, what it moved, and how it
 * ended.
 *
 * A run recorded by a binary rolled back past the migration that named runs
 * has no reference, so the reference alone does not name a row — the caller's
 * `key` is which half finished when, plus the reference, since the two halves
 * never run at once and no two rows share both.
 */
export function SyncRunRow({ run, label }: { run: SyncRun; label: string }) {
  return (
    <li
      className="grid gap-1 rounded-lg border border-[var(--rule)] p-3 text-sm sm:grid-cols-[1fr_auto]"
      data-phase={run.phase}
    >
      <span className="text-[var(--ink-2)]">{formatTimestamp(run.completedAt)}</span>
      <span className="font-medium">{label}</span>
      <span className="text-[var(--ink-2)]">{runCounts(run)}</span>
      <span className="flex flex-wrap items-center gap-2 sm:row-span-2 sm:col-start-2 sm:row-start-1 sm:self-center">
        <Badge className={TONE_CLASSES[runTone(run)]} variant="secondary">
          {runResult(run)}
        </Badge>
        {/*
         * The reference is the only thing on the row that is not about what
         * happened. It is here so a notification can be traced to the run it
         * was sent for, and it is presented as the opaque string it is rather
         * than dressed up as an identifier. A run recorded before runs were
         * named has none to show.
         */}
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
