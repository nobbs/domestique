/**
 * What each Wahoo account holds.
 *
 * This is the question an operator actually has after a sync: not "did the run
 * succeed" but "is what I planned now on both accounts". The two are different —
 * a run that wrote one account and could not write the other is recorded once,
 * as failed, and says nothing about which account is behind.
 *
 * It is deliberately a claim about the Wahoo account and nothing further:
 * whether a head unit has since downloaded those routes is not something the
 * service can see, so the card says so rather than letting "up to date" be read
 * as "on the device".
 */

import { useQuery } from "@tanstack/react-query";
import { statusQuery } from "../../api/queries";
import type { TargetStatus } from "../../api/types";
import { formatTimestamp } from "../../lib/format";
import { GUIDANCE_LABELS, syncGuidance } from "../../lib/syncGuidance";
import { authorisationGuidance, authorisationStartHref } from "../../lib/targetAuthorisation";

/**
 * What one account holds, in a line.
 *
 * An account that is not connected holds whatever it was last written, but
 * saying so alongside "not connected" would be two answers to one question. The
 * connection is the answer that matters, so it is the one the line gives.
 */
export function stagesSummary(target: TargetStatus): string {
  const authorisation = authorisationGuidance(target.authorisation);
  if (authorisation) {
    return authorisation.label;
  }

  const held =
    target.stages.pending === 0
      ? `All ${target.stages.current} ${target.stages.current === 1 ? "route" : "routes"}`
      : `${target.stages.current} of ${target.stages.current + target.stages.pending} routes`;
  const written = target.lastRun ? `written ${formatTimestamp(target.lastRun.completedAt)}` : null;

  return written ? `${held} · ${written}` : held;
}

/**
 * How this account's last write ended, when it did not end well.
 *
 * The service reduces every unsuccessful run to `failed` in its own one word,
 * because that word answers a different question — whether this account is
 * behind — and a held gate leaves it behind either way. Here there is room to
 * say which, and a gate must not be read as a fault: the account is intact and
 * the next move is the operator's.
 */
export function lastRunSummary(target: TargetStatus): string | null {
  const guidance = targetGuidance(target);
  if (!guidance || !target.lastRun) {
    return null;
  }

  return `${GUIDANCE_LABELS[guidance.kind]} · ${formatTimestamp(target.lastRun.completedAt)}`;
}

/** One account's last write, explained, or nothing when it succeeded. */
function targetGuidance(target: TargetStatus) {
  return target.lastRun
    ? syncGuidance("targets", target.lastRun.result, target.lastRun.failure)
    : undefined;
}

/** The body of the "What the accounts hold" card: one row per account. */
export function TargetConvergence() {
  const { data, isPending, isError } = useQuery(statusQuery());

  if (isPending) {
    return null;
  }
  if (isError) {
    return <p className="sync-card__error">The service did not say what the accounts hold.</p>;
  }

  return (
    <>
      <ul className="sync-card__list">
        {data.targets.map((target) => {
          const guidance = targetGuidance(target);
          const authorisation = authorisationGuidance(target.authorisation);
          const failure = lastRunSummary(target);

          return (
            <li
              className="sync-row"
              data-convergence={target.convergence}
              data-run={guidance?.kind}
              key={target.id}
            >
              <div className="sync-row__text">
                <span className="sync-row__title">{target.id}</span>
                <span className="sync-row__detail">{stagesSummary(target)}</span>
                {failure ? <span className="sync-row__detail">{failure}</span> : null}
                {authorisation ? (
                  <span className="sync-guidance" data-kind="blocked">
                    {authorisation.detail}
                  </span>
                ) : null}
                {guidance ? (
                  <span className="sync-guidance" data-kind={guidance.kind}>
                    <strong>{guidance.headline}</strong> {guidance.remediation}
                  </span>
                ) : null}
              </div>
              {authorisation?.action ? (
                <div className="sync-row__actions">
                  {/*
                   * A plain anchor, and a full-page navigation: the flow leaves
                   * this application for Wahoo and returns to it, so there is
                   * nothing here for a client-side route or a background request
                   * to carry. It is a GET the service gates on the same identity
                   * as this page.
                   */}
                  <a className="sync-row__connect" href={authorisationStartHref(target.id)}>
                    {authorisation.action} {target.id}
                  </a>
                </div>
              ) : null}
            </li>
          );
        })}
      </ul>
      <p className="sync-card__foot">
        This is what the accounts hold, not what a head unit has downloaded.
      </p>
    </>
  );
}
