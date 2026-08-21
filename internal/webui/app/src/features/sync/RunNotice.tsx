/**
 * The run a notification was sent about, at the top of the page.
 *
 * A Pushover message carries one opaque reference and lands here as
 * `/sync?run=…`. The operator arrives having been told something needs them,
 * possibly hours later, and the card they land on names the run they were told
 * about rather than making them match a reference against the history below.
 *
 * It also appears without a reference when the last run of either half still
 * needs attention, because a page that shows a held gate three cards down and
 * says nothing at the top has buried the only thing on it that is not
 * information.
 *
 * Nothing here is written by this page: the headline and the remediation come
 * from `syncGuidance`, which turns the service's safe failure category into
 * words. A reference is printed as the opaque string it is.
 */

import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { Link } from "react-router";
import { triggerSync } from "../../api/client";
import { statusQuery, syncRunsQuery } from "../../api/queries";
import type { Status, SyncPhase, SyncRun } from "../../api/types";
import { SYNC_PHASES } from "../../api/types";
import { Button } from "../../components/Button";
import { formatTimestamp } from "../../lib/format";
import { syncGuidance } from "../../lib/syncGuidance";

/** What the notice is about: one run, in the terms guidance can read. */
export interface NoticeRun {
  phase: SyncPhase;
  completedAt: string;
  result: string;
  failure?: string | undefined;
  /** Empty for a run recorded before runs were named. */
  reference: string;
}

/**
 * Which run this page should be about.
 *
 * A reference names one exactly, and it is honoured whatever it says — an
 * operator following a notification about a run that succeeded should still land
 * on that run rather than on whatever went wrong since. Without one, the notice
 * is only for a half whose last run still needs something, and reading comes
 * before writing because a write on a stale library is the next thing to go
 * wrong.
 */
export function noticeRun(
  reference: string | null,
  runs: SyncRun[],
  status: Status | undefined,
): NoticeRun | null {
  if (reference !== null) {
    return runs.find((run) => run.reference === reference) ?? null;
  }
  if (!status) {
    return null;
  }

  for (const phase of SYNC_PHASES) {
    const run = status.sync.phases[phase];
    if (run && syncGuidance(phase, run.lastResult, run.lastFailure)) {
      return {
        phase,
        completedAt: run.lastCompletedAt,
        result: run.lastResult,
        failure: run.lastFailure,
        // The status carries no reference, only the last result of a half. The
        // matching row in the history has one, and it is one card away.
        reference: "",
      };
    }
  }

  return null;
}

/** What pressing the one action here would do, in its own words. */
const RETRY_LABELS: Record<SyncPhase, string> = {
  source: "Run the read again",
  targets: "Run the write again",
};

export function RunNotice({ reference }: { reference: string | null }) {
  const queryClient = useQueryClient();
  const status = useQuery(statusQuery());
  // The history card below has already asked for this, so on an ordinary visit
  // this is the cache. A notification tap arrives with nothing cached and this
  // is the request that resolves the reference.
  const history = useInfiniteQuery(syncRunsQuery());
  const run = useMutation({
    mutationFn: (phase: SyncPhase) => triggerSync(phase),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
  });

  const runs = history.data?.pages.flatMap((page) => page.runs) ?? [];
  const notice = noticeRun(reference, runs, status.data);

  /*
   * The history is read a page at a time, and a notification can be older than
   * the first page of it. A reference the loaded pages do not hold is therefore
   * not yet an answer: keep asking for the next page until the run is found or
   * the history runs out. A page that fails to arrive ends the walk — retrying
   * it here would be a loop rather than a search.
   */
  const { fetchNextPage, hasNextPage, isFetchingNextPage, isFetchNextPageError } = history;
  const searching = hasNextPage && !isFetchNextPageError;
  useEffect(() => {
    if (reference === null || notice || !searching || isFetchingNextPage) {
      return;
    }
    void fetchNextPage();
  }, [reference, notice, searching, isFetchingNextPage, fetchNextPage]);

  /*
   * A reference the history no longer holds is the ordinary end of a pruned
   * run, not a fault: the notification outlived what it pointed at. That is
   * only true once the history has actually been read, though — a history the
   * service could not answer for says nothing about the run, so the two are
   * told apart rather than both reported as a pruning.
   */
  if (!notice) {
    if (reference === null || history.isPending || searching) {
      return null;
    }

    // Either the history never arrived, or the walk back through it stopped on
    // a page that did not. Both leave the reference unchecked.
    const unread = !history.isSuccess || isFetchNextPageError;

    return (
      <section className="panel sync-card run-notice" aria-labelledby="notice-heading">
        <h2 className="sync-card__heading" id="notice-heading">
          {unread ? "That run could not be looked up" : "That run is no longer kept"}
        </h2>
        <p className="sync-card__line">
          {unread ? (
            <>
              The notification named run {reference}, and the history it would be found in could not
              be read. The error is below.
            </>
          ) : (
            <>
              The notification named run {reference}, which has been pruned since it was sent. What
              has happened since is below.
            </>
          )}
        </p>
      </section>
    );
  }

  const guidance = syncGuidance(notice.phase, notice.result, notice.failure);
  const headline = guidance ? guidance.headline : "That run finished";
  const tone = guidance ? (guidance.kind === "blocked" ? "hold" : "alert") : "good";

  return (
    <section
      className="panel sync-card run-notice"
      data-tone={tone}
      aria-labelledby="notice-heading"
    >
      <h2 className="sync-card__heading" id="notice-heading">
        {headline}
      </h2>
      <p className="run-notice__reference">
        {formatTimestamp(notice.completedAt)}
        {notice.reference === "" ? null : <> · run {notice.reference}</>}
      </p>
      <p className="sync-card__line">
        {guidance ? guidance.remediation : "Nothing needs doing. It is in the history below."}
      </p>
      <div className="run-notice__actions">
        <Button variant="primary" disabled={run.isPending} onClick={() => run.mutate(notice.phase)}>
          {RETRY_LABELS[notice.phase]}
        </Button>
        {/*
         * The other way out. Whatever the remediation asks for beyond running
         * the half again — a limit raised, an account reconnected — is settled
         * in the service's own configuration or in the cards below, so the
         * second action leaves rather than pretending this card can do it.
         */}
        <Link className="run-notice__dismiss" to="/sync">
          Dismiss
        </Link>
      </div>
      {run.isError ? (
        <p className="sync-card__error" role="alert">
          {run.error instanceof Error && run.error.message
            ? run.error.message
            : "That run could not be started."}
        </p>
      ) : null}
    </section>
  );
}
