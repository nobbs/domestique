/**
 * The runs the service has recorded, newest first.
 *
 * The controls above this say what each half last came to; this says what the
 * halves have been doing. It is the section that answers "was that failure the
 * first one, or the fifth this week", which no single last result can.
 *
 * It is recent history rather than a record: the service prunes old runs, so a
 * page reaches back as far as what is still kept and no further. Each row is the
 * aggregate the service recorded and nothing else — no route, no geometry, no
 * account — plus the opaque reference a Pushover message carries, which is what
 * an operator matches a notification against a row with.
 */

import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { statusQuery, syncRunsQuery } from "../../api/queries";
import type { SyncPhase, SyncRun } from "../../api/types";
import { Button } from "../../components/Button";
import { ErrorMessage } from "../../components/StatusMessage";
import { formatTimestamp } from "../../lib/format";
import { GUIDANCE_LABELS, syncGuidance } from "../../lib/syncGuidance";

/** Which half a row was, in the words the controls above it use. */
const PHASE_LABELS: Record<SyncPhase, string> = {
  source: "Read from VeloPlanner",
  targets: "Write to Wahoo",
};

/** How a recorded run ended, in one phrase. */
export function runResult(run: SyncRun): string {
  const guidance = syncGuidance(run.phase, run.result, run.failure);

  return guidance ? GUIDANCE_LABELS[guidance.kind] : "Succeeded";
}

/**
 * What a recorded run moved.
 *
 * A read is measured by what it found and a write by what it changed, which is
 * the same split the controls above use — the other half's counts are nought on
 * every row and would read as work that did not happen.
 */
export function runCounts(run: SyncRun): string {
  return run.phase === "source"
    ? `${run.sourceStages} stages`
    : `${run.created} created · ${run.updated} updated · ${run.deleted} deleted`;
}

export function SyncHistory() {
  const queryClient = useQueryClient();
  const { data: status } = useQuery(statusQuery());
  const { data, isPending, isError, error, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useInfiniteQuery(syncRunsQuery());

  // A run that finishes writes a row this list does not know about. The status
  // above is already polled while a run is in flight, so the instant it reports
  // a newer completion is the instant there is something new to read here.
  //
  // A change is what triggers the refresh, not a value: the first completion
  // this component sees is the one the page was loaded with, and re-asking for
  // a page that has just been fetched would be asking on nobody's behalf.
  const lastCompletedAt = status?.sync.lastCompletedAt;
  const seenCompletion = useRef(lastCompletedAt);
  useEffect(() => {
    if (seenCompletion.current === lastCompletedAt) {
      return;
    }
    seenCompletion.current = lastCompletedAt;
    void queryClient.invalidateQueries({ queryKey: syncRunsQuery().queryKey });
  }, [lastCompletedAt, queryClient]);

  if (isPending) {
    return null;
  }
  if (isError) {
    return <ErrorMessage what="the run history" error={error} />;
  }

  const runs = data.pages.flatMap((page) => page.runs);

  return (
    <section className="history" aria-labelledby="history-heading">
      <h2 className="history__heading" id="history-heading">
        Recent runs
      </h2>
      {runs.length === 0 ? (
        <p className="history__empty">Nothing has run yet.</p>
      ) : (
        <>
          <ul className="history__runs">
            {runs.map((run) => (
              <li className="history__run" data-phase={run.phase} key={run.reference}>
                <div className="history__text">
                  <span className="history__phase">{PHASE_LABELS[run.phase]}</span>
                  <span className="history__counts">{runCounts(run)}</span>
                  {/*
                   * The reference is the only thing on the row that is not about
                   * what happened. It is here so a notification can be traced to
                   * the run it was sent for, and it is presented as the opaque
                   * string it is rather than dressed up as an identifier.
                   */}
                  <span className="history__reference">Run {run.reference}</span>
                </div>
                <div className="history__outcome">
                  <span className="history__result">{runResult(run)}</span>
                  <span className="history__when">{formatTimestamp(run.completedAt)}</span>
                </div>
              </li>
            ))}
          </ul>
          {hasNextPage ? (
            <Button
              variant="quiet"
              className="history__more"
              disabled={isFetchingNextPage}
              onClick={() => void fetchNextPage()}
            >
              Show earlier runs
            </Button>
          ) : (
            <p className="history__end">
              That is every run still kept. Older ones are pruned as new runs are recorded.
            </p>
          )}
        </>
      )}
    </section>
  );
}
