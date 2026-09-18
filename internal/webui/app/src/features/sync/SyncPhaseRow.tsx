import { IconPlayerPlay } from "@tabler/icons-react";
import type { SyncPhase, SyncPhaseRun } from "../../api/types";
import { Button } from "../../components/Button";
import { InsetRow, RowNote, type RowTone } from "../../components/InsetList";
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
      ? `${run.sourceRoutes} routes`
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
  /** How often the schedule starts this half, in the words the switch shows. */
  cadence: string;
  scheduleDisabled: boolean;
  onToggle: () => void;
  running: boolean;
  /** Absent hides the button: running either half now is an admin action. */
  onRun: (() => void) | undefined;
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
  cadence,
  scheduleDisabled,
  onToggle,
  running,
  onRun,
}: SyncPhaseRowProps) {
  const guidance = lastRun
    ? syncGuidance(phase, lastRun.lastResult, lastRun.lastFailure)
    : undefined;

  const tone: RowTone = !lastRun
    ? "quiet"
    : guidance
      ? guidance.kind === "blocked"
        ? "hold"
        : "alert"
      : "good";

  return (
    <InsetRow
      tone={tone}
      toneLabel={!lastRun ? "Not run yet" : guidance ? guidance.headline : "Last run succeeded"}
      title={label}
      detail={runSummary(phase, lastRun)}
      actions={
        <>
          {/* Both rows look alike, so the accessible name is what tells the two switches apart. */}
          <span className="flex items-center gap-2 text-[var(--ink-2)] text-xs">
            {cadence}
            <Switch
              checked={enabled}
              disabled={scheduleDisabled}
              onCheckedChange={onToggle}
              aria-label={`${cadence}: ${label}`}
            />
          </span>
          {onRun ? (
            <Button
              variant="outline"
              disabled={running}
              onClick={onRun}
              aria-label={`Run now: ${label}`}
              icon={running ? null : <IconPlayerPlay />}
            >
              {running ? <Spinner aria-label={`Running ${label}`} /> : null}
            </Button>
          ) : null}
        </>
      }
      note={
        /*
         * A gate that held is not an error the operator caused, so it is
         * stated rather than announced: the page is being read, not
         * interrupted, and the run it describes finished some time ago.
         */
        guidance ? (
          <RowNote tone={tone} data-kind={guidance.kind}>
            <strong>{guidance.headline}</strong> {guidance.remediation}
          </RowNote>
        ) : null
      }
    />
  );
}
