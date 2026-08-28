import type { TargetStatus } from "../../api/types";
import { Button } from "../../components/Button";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "../../components/ui/alert-dialog";
import { Input } from "../../components/ui/input";
import { Spinner } from "../../components/ui/spinner";
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
function stagesSummary(target: TargetStatus): string {
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
function lastRunSummary(target: TargetStatus): string | null {
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

export interface AccountRowProps {
  target: TargetStatus;
  reconciling: boolean;
  onReconcile: () => void;
  /**
   * The delete-confirmation dialog's state, held by the caller rather than in
   * this row: only one account's confirmation may be open at a time, which a
   * row cannot promise on its own.
   */
  clear: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    confirmation: string;
    onConfirmationChange: (value: string) => void;
    pending: boolean;
    onConfirm: () => void;
  };
}

/** One account: what it holds, and the two ways to act on it. */
export function AccountRow({ target, reconciling, onReconcile, clear }: AccountRowProps) {
  const guidance = targetGuidance(target);
  const authorisation = authorisationGuidance(target.authorisation);
  const failure = lastRunSummary(target);

  return (
    <li
      className="flex flex-col gap-3 rounded-lg border border-[var(--rule)] p-3 sm:flex-row sm:items-start sm:justify-between"
      data-convergence={target.convergence}
      data-run={guidance?.kind}
    >
      <div className="flex min-w-0 flex-col gap-1 text-sm">
        <span className="font-semibold">{target.id}</span>
        <span className="text-[var(--ink-2)]">{stagesSummary(target)}</span>
        {failure ? <span className="text-[var(--ink-2)]">{failure}</span> : null}
        {authorisation ? (
          <span className="text-[var(--hold)]" data-kind="blocked">
            {authorisation.detail}
          </span>
        ) : null}
        {guidance ? (
          <span
            className={guidance.kind === "blocked" ? "text-[var(--hold)]" : "text-[var(--alert)]"}
            data-kind={guidance.kind}
          >
            <strong>{guidance.headline}</strong> {guidance.remediation}
          </span>
        ) : null}
      </div>
      {authorisation?.action ? (
        <div className="flex shrink-0 items-center gap-2">
          {/*
           * A plain anchor, and a full-page navigation: the flow leaves this
           * application for Wahoo and returns to it, so there is nothing here
           * for a client-side route or a background request to carry. It is
           * a GET the service gates on the same identity as this page.
           */}
          <a
            className="inline-flex h-8 items-center rounded-lg bg-[var(--accent)] px-3 text-sm font-semibold text-white hover:brightness-95 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
            href={authorisationStartHref(target.id)}
          >
            {authorisation.action} {target.id}
          </a>
        </div>
      ) : !authorisation ? (
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {/*
           * While a deletion is being confirmed it is the only thing this
           * row offers. Leaving the ordinary action beside it would crowd
           * the sentence that explains what is about to happen, and put a
           * button next to it that undoes the point of asking.
           */}
          {/*
           * "This account", not "this device": the button reconciles the
           * Wahoo account the row is about, and says so, because what it
           * presses is not the same thing as a head unit fetching routes
           * from it on its own schedule.
           */}
          <Button
            variant="standard"
            disabled={reconciling}
            onClick={onReconcile}
            aria-label={`Reconcile now: ${target.id}`}
          >
            {reconciling ? <Spinner aria-label="Reconciling" /> : null}
            Reconcile this account
          </Button>
          {/*
           * Deleting everything is not a variant of reconciling, so it does
           * not sit beside it as an equal. It asks first, and what it asks
           * for is the account's own name — the one confirmation a stray
           * click cannot supply.
           */}
          <AlertDialog open={clear.open} onOpenChange={clear.onOpenChange}>
            <AlertDialogTrigger
              className="text-sm text-[var(--alert)] underline-offset-4 hover:underline disabled:opacity-50"
              disabled={clear.pending}
            >
              Delete all routes…
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete Domestique routes from {target.id}?</AlertDialogTitle>
                <AlertDialogDescription>
                  Routes you made yourself are left alone. The next sync puts these back, and no
                  other sync starts until this clear finishes.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <label className="grid gap-1 text-sm" htmlFor={`clear-${target.id}`}>
                Type <strong>{target.id}</strong> to confirm.
                <Input
                  id={`clear-${target.id}`}
                  value={clear.confirmation}
                  onChange={(event) => clear.onConfirmationChange(event.target.value)}
                  autoComplete="off"
                />
              </label>
              <AlertDialogFooter>
                {/*
                 * The application's own button, handed to the primitive rather
                 * than reached for inside it: `ui/alert-dialog` stays the
                 * vendored file the registry wrote, and this file decides what
                 * its footer is made of.
                 */}
                <AlertDialogCancel render={<Button variant="standard" />}>Cancel</AlertDialogCancel>
                <Button
                  variant="danger"
                  disabled={clear.confirmation !== target.id || clear.pending}
                  onClick={clear.onConfirm}
                  aria-label={`Delete every Domestique route from ${target.id}`}
                >
                  {clear.pending ? <Spinner aria-label="Deleting" /> : null}
                  Delete them
                </Button>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      ) : null}
    </li>
  );
}
