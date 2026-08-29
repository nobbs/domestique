import type { SyncPhase, SyncPhaseRun } from "../../api/types";
import { Button } from "../../components/Button";
import { Spinner } from "../../components/ui/spinner";
import { Switch } from "../../components/ui/switch";
import { formatTimestamp } from "../../lib/format";
import { syncGuidance } from "../../lib/syncGuidance";

/**
 * What one half's last run amounts to, in a line.
 *
 * A run that did not succeed reduces to how it ended and when. What it means
 * and what to do about it is the guidance line beneath it, because "held" and
 * "failed" ask opposite things of an operator and neither fits in a count.
 */
function runSummary(phase: SyncPhase, run: SyncPhaseRun | undefined): string {
  if (!run) {
    return "Has not run yet";
  }
  const when = formatTimestamp(run.lastCompletedAt);
  const guidance = syncGuidance(phase, run.lastResult, run.lastFailure);
  if (guidance) {
    return `${when} · ${guidance.kind === "blocked" ? "held by a gate" : "did not finish"}`;
  }
  const counts =
    phase === "source"
      ? `${run.sourceStages} routes`
      : [
          `${run.created} created`,
          `${run.updated} updated`,
          ...(run.deleted > 0 ? [`${run.deleted} deleted`] : []),
        ].join(", ");

  return `${when} · ${counts}`;
}

export interface SyncPhaseRowProps {
  phase: SyncPhase;
  label: string;
  lastRun: SyncPhaseRun | undefined;
  enabled: boolean;
  scheduleDisabled: boolean;
  onToggle: () => void;
  running: boolean;
  onRun: () => void;
}

/**
 * One half — the read or the write — as a row: what it last came to, its
 * hourly switch, and the button that runs it now regardless of the switch.
 *
 * The button never disables itself for a switched-off half: turning the
 * schedule off is a statement about unattended runs, not a lock.
 */
export function SyncPhaseRow({
  phase,
  label,
  lastRun,
  enabled,
  scheduleDisabled,
  onToggle,
  running,
  onRun,
}: SyncPhaseRowProps) {
  const guidance = lastRun
    ? syncGuidance(phase, lastRun.lastResult, lastRun.lastFailure)
    : undefined;

  return (
    <li className="flex flex-col gap-3 rounded-lg border border-[var(--rule)] p-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="flex min-w-0 flex-col gap-1 text-sm">
        <span className="font-semibold">{label}</span>
        <span className="text-[var(--ink-2)]">{runSummary(phase, lastRun)}</span>
        {/*
         * A gate that held is not an error the operator caused, so it is
         * stated rather than announced: the page is being read, not
         * interrupted, and the run it describes finished some time ago.
         */}
        {guidance ? (
          <span
            className={guidance.kind === "blocked" ? "text-[var(--hold)]" : "text-[var(--alert)]"}
            data-kind={guidance.kind}
          >
            <strong>{guidance.headline}</strong> {guidance.remediation}
          </span>
        ) : null}
      </div>
      <div className="flex shrink-0 flex-wrap items-center gap-3">
        {/*
         * Both rows carry the same two words, so the visible text alone
         * names neither half. The accessible name says which one, since a
         * reader arriving at the second switch has no row above it to tell
         * them apart. The interval is the service's own and is fixed at an
         * hour, so the switch can say what it schedules.
         */}
        <div className="flex items-center gap-2 text-sm">
          <Switch
            checked={enabled}
            disabled={scheduleDisabled}
            onCheckedChange={onToggle}
            aria-label={`Hourly: ${label}`}
          />
          <span>Hourly</span>
        </div>
        <Button
          variant="outline"
          disabled={running}
          onClick={onRun}
          aria-label={`Run now: ${label}`}
        >
          {running ? <Spinner aria-label={`Running ${label}`} /> : null}
          Run now
        </Button>
      </div>
    </li>
  );
}
