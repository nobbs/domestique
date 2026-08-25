/**
 * The runs the service has recorded, newest first.
 *
 * The card above this says what each half last came to; this says what the
 * halves have been doing. It is the card that answers "was that failure the
 * first one, or the fifth this week", which no single last result can.
 *
 * It is recent history rather than a record: the service prunes old runs, so a
 * page reaches back as far as what is still kept and no further. Each row is the
 * aggregate the service recorded and nothing else — no route, no geometry, no
 * account — plus the opaque reference a Pushover message carries, which is what
 * an operator matches a notification against a row with.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { statusQuery, syncRunsQueryKey, useSyncRuns, webUIConfigQuery } from "../../api/queries";
import type { SyncRun } from "../../api/types";
import { Badge } from "../../components/ui/badge";
import { Button } from "../../components/ui/button";
import { Skeleton } from "../../components/ui/skeleton";
import { Spinner } from "../../components/ui/spinner";
import { formatTimestamp } from "../../lib/format";
import { GUIDANCE_LABELS, syncGuidance } from "../../lib/syncGuidance";
import { phaseLabels } from "../../lib/syncLabels";

/** How a recorded run ended, in one phrase. */
export function runResult(run: SyncRun): string {
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
export function runTone(run: SyncRun): "good" | "hold" | "alert" {
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
export function runCounts(run: SyncRun): string {
  if (run.phase === "source") {
    return `${run.sourceStages} routes`;
  }

  return [
    `${run.created} created`,
    `${run.updated} updated`,
    ...(run.deleted > 0 ? [`${run.deleted} deleted`] : []),
  ].join(" · ");
}

/** What names one row apart from the others; see where it is used. */
function runKey(run: SyncRun): string {
  return `${run.phase}-${run.completedAt}-${run.reference}`;
}

/** The body of the "What has happened" card: one row per recorded run. */
export function SyncHistory() {
  const queryClient = useQueryClient();
  const { data: status } = useQuery(statusQuery());
  const config = useQuery(webUIConfigQuery());
  const labels = phaseLabels(Object.keys(config.data?.sourceBaseUrls ?? {}));
  const { data, isPending, isError, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useSyncRuns();

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
    void queryClient.invalidateQueries({ queryKey: syncRunsQueryKey() });
  }, [lastCompletedAt, queryClient]);

  if (isPending) {
    return <Skeleton className="h-24 w-full" role="status" aria-label="Loading sync history" />;
  }
  if (isError) {
    return (
      <p className="text-sm text-[var(--alert)]">The service did not say what has happened.</p>
    );
  }

  const runs = data.pages.flatMap((page) => page.runs);
  if (runs.length === 0) {
    return <p className="text-sm text-[var(--ink-2)]">Nothing has run yet.</p>;
  }

  return (
    <>
      <ul className="grid gap-2">
        {runs.map((run) => (
          /*
           * A run recorded by a binary rolled back past the migration that named
           * runs has no reference, so the reference alone does not name a row.
           * Which half finished when is the rest of the name: the two halves
           * never run at once, so no two rows share both.
           */
          <li
            className="grid gap-1 rounded-lg border border-[var(--rule)] p-3 text-sm sm:grid-cols-[1fr_auto]"
            data-phase={run.phase}
            key={runKey(run)}
          >
            <span className="text-[var(--ink-2)]">{formatTimestamp(run.completedAt)}</span>
            <span className="font-medium">{labels[run.phase]}</span>
            <span className="text-[var(--ink-2)]">{runCounts(run)}</span>
            <span className="flex flex-wrap items-center gap-2 sm:row-span-2 sm:col-start-2 sm:row-start-1 sm:self-center">
              <Badge
                className={
                  runTone(run) === "good"
                    ? "border-[var(--good)] bg-[var(--base)] text-[var(--ink)]"
                    : runTone(run) === "hold"
                      ? "border-[var(--hold)] bg-[var(--base)] text-[var(--ink)]"
                      : "border-[var(--alert)] bg-[var(--base)] text-[var(--ink)]"
                }
                variant="secondary"
              >
                {runResult(run)}
              </Badge>
              {/*
               * The reference is the only thing on the row that is not about
               * what happened. It is here so a notification can be traced to the
               * run it was sent for, and it is presented as the opaque string it
               * is rather than dressed up as an identifier. A run recorded before
               * runs were named has none to show.
               */}
              {run.reference === "" ? null : (
                <span className="text-xs text-[var(--ink-2)]">
                  <span className="sr-only">Run reference </span>
                  {run.reference}
                </span>
              )}
            </span>
          </li>
        ))}
      </ul>
      {hasNextPage ? (
        <Button
          variant="outline"
          disabled={isFetchingNextPage}
          onClick={() => void fetchNextPage()}
        >
          {isFetchingNextPage ? <Spinner aria-label="Loading earlier runs" /> : null}
          Earlier runs
        </Button>
      ) : (
        <p className="text-sm text-[var(--ink-2)]">
          Older runs are pruned as new ones are recorded.
        </p>
      )}
    </>
  );
}
