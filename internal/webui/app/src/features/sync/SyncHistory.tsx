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
import { Button } from "../../components/Button";
import { Skeleton } from "../../components/ui/skeleton";
import { Spinner } from "../../components/ui/spinner";
import { phaseLabels } from "../../lib/syncLabels";
import { SyncRunRow } from "./SyncRunRow";

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
          <SyncRunRow key={runKey(run)} run={run} label={labels[run.phase]} />
        ))}
      </ul>
      {hasNextPage ? (
        <Button
          variant="standard"
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
