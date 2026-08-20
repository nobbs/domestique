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
import { GUIDANCE_LABELS, syncGuidance } from "../../lib/syncGuidance";
import { authorisationGuidance, authorisationStartHref } from "../../lib/targetAuthorisation";

const CONVERGENCE_LABELS: Record<Convergence, string> = {
  current: "Up to date",
  lagging: "Behind",
  failed: "Last write failed",
  unauthorized: "Not connected",
};

/**
 * What one account's counts amount to, in a sentence.
 *
 * An account that is not connected gets the counts like any other. What being
 * unconnected means, and what to do about it, is the authorisation guidance
 * beside them, which is more specific than a sentence here could be.
 */
export function stagesSummary(target: TargetStatus): string {
  if (target.stages.pending === 0) {
    return `All ${target.stages.current} ${target.stages.current === 1 ? "stage" : "stages"} written.`;
  }

  return `${target.stages.current} written · ${target.stages.pending} outstanding`;
}

/**
 * What one account's own last reconciliation amounts to, in a sentence.
 *
 * Every run reported here is the writing half, so its guidance is read against
 * that phase whatever else the page is showing.
 */
export function lastRunSummary(target: TargetStatus): string {
  if (!target.lastRun) {
    return "Has not been written to yet.";
  }
  const when = formatTimestamp(target.lastRun.completedAt);
  const guidance = targetGuidance(target);
  if (!guidance) {
    return `Last written ${when}`;
  }

  return `${GUIDANCE_LABELS[guidance.kind]} · ${when}`;
}

/** One account's last reconciliation, explained, or nothing when it succeeded. */
function targetGuidance(target: TargetStatus) {
  return target.lastRun
    ? syncGuidance("targets", target.lastRun.result, target.lastRun.failure)
    : undefined;
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
        {data.targets.map((target) => {
          const guidance = targetGuidance(target);
          const authorisation = authorisationGuidance(target.authorisation);
          // The service reduces every unsuccessful run to `failed` in its one
          // word, because that word answers a different question — whether this
          // account is behind — and a held gate leaves it behind either way.
          // Here there is room to say which, and a gate must not be read as a
          // fault: the account is intact and the next move is the operator's.
          //
          // Authorisation wins over both, because an account that cannot be
          // written to at all has nothing to say about how far behind it is.
          const state =
            authorisation?.label ??
            (guidance?.kind === "blocked"
              ? GUIDANCE_LABELS.blocked
              : CONVERGENCE_LABELS[target.convergence]);

          return (
            <li
              className="convergence__target"
              data-convergence={target.convergence}
              data-run={guidance?.kind}
              key={target.id}
            >
              <div className="convergence__text">
                <span className="convergence__id">{target.id}</span>
                <span className="convergence__stages">{stagesSummary(target)}</span>
                <span className="convergence__run">{lastRunSummary(target)}</span>
                {authorisation ? (
                  <span className="convergence__authorisation">
                    {authorisation.detail}{" "}
                    {authorisation.action ? (
                      /*
                       * A plain anchor, and a full-page navigation: the flow
                       * leaves this application for Wahoo and returns to it, so
                       * there is nothing here for a client-side route or a
                       * background request to carry. It is a GET the service
                       * gates on the same identity as this page.
                       */
                      <a
                        className="convergence__connect"
                        href={authorisationStartHref(target.id)}
                        // The slot is in the name because two accounts sit in
                        // this list and "Connect" alone would not say which.
                        aria-label={`${authorisation.action} ${target.id} to Wahoo`}
                      >
                        {authorisation.action}
                      </a>
                    ) : null}
                  </span>
                ) : null}
                {guidance ? (
                  <span className="convergence__guidance" data-kind={guidance.kind}>
                    <span className="convergence__guidance-headline">{guidance.headline}</span>{" "}
                    {guidance.remediation}
                  </span>
                ) : null}
              </div>
              <span className="convergence__state">{state}</span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
