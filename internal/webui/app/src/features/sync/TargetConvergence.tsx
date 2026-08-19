/**
 * Whether each Wahoo account holds the library.
 *
 * This is the question the operator actually has after a sync: not "did the run
 * succeed" but "is what I planned now on both accounts". The two are different —
 * a run that wrote one account and could not write the other is recorded once,
 * as failed, and says nothing about which account is behind.
 *
 * Everything here is derived from the service's own state, and it is deliberately
 * a claim about the Wahoo account and nothing further: whether a head unit has
 * since downloaded those routes is not something the service can see, so the
 * section says so rather than letting "current" be read as "on the device".
 */

import { useQuery } from "@tanstack/react-query";
import { statusQuery } from "../../api/queries";
import type { TargetConvergence as Convergence, TargetStatus } from "../../api/types";
import { ErrorMessage } from "../../components/StatusMessage";
import { formatTimestamp } from "../../lib/format";

const CONVERGENCE_LABELS: Record<Convergence, string> = {
  current: "Up to date",
  lagging: "Behind",
  failed: "Last write failed",
  unauthorized: "Not connected",
};

/** What one account's counts amount to, in a sentence. */
export function stagesSummary(target: TargetStatus): string {
  if (target.convergence === "unauthorized") {
    return "Waiting to be connected to Wahoo. Nothing can be written until it is.";
  }
  if (target.stages.pending === 0) {
    return `All ${target.stages.current} ${target.stages.current === 1 ? "stage" : "stages"} written.`;
  }

  return `${target.stages.current} written · ${target.stages.pending} outstanding`;
}

/** What one account's own last reconciliation amounts to, in a sentence. */
export function lastRunSummary(target: TargetStatus): string {
  if (!target.lastRun) {
    return "Has not been written to yet.";
  }
  const when = formatTimestamp(target.lastRun.completedAt);
  if (target.lastRun.result === "succeeded") {
    return `Last written ${when}`;
  }

  return `${target.lastRun.result}${target.lastRun.failure ? ` (${target.lastRun.failure})` : ""} · ${when}`;
}

export function TargetConvergence() {
  const { data, isPending, isError, error } = useQuery(statusQuery());

  if (isPending) {
    return null;
  }
  if (isError) {
    return <ErrorMessage what="target convergence" error={error} />;
  }

  return (
    <section className="convergence" aria-labelledby="convergence-heading">
      <h2 className="convergence__heading" id="convergence-heading">
        Wahoo accounts
      </h2>
      <p className="convergence__scope">
        {data.converged
          ? "Every stored stage is on every account, at the revision stored here."
          : "Some stored stages are not yet on every account."}{" "}
        This is what the accounts hold, not what a head unit has downloaded — a device fetches
        routes on its own schedule.
      </p>
      <ul className="convergence__targets">
        {data.targets.map((target) => (
          <li className="convergence__target" data-convergence={target.convergence} key={target.id}>
            <div className="convergence__text">
              <span className="convergence__id">{target.id}</span>
              <span className="convergence__stages">{stagesSummary(target)}</span>
              <span className="convergence__run">{lastRunSummary(target)}</span>
            </div>
            <span className="convergence__state">{CONVERGENCE_LABELS[target.convergence]}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
