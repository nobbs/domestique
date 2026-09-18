import { IconRefresh } from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { TargetStatus } from "../../api/types";
import { Button } from "../../components/Button";
import { InsetRow, RowNote, type RowTone } from "../../components/InsetList";
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
import { useEffectiveAdmin } from "../../lib/identity";
import { GUIDANCE_LABELS, syncGuidance } from "../../lib/syncGuidance";
import { authorisationGuidance, authorisationStartHref } from "../../lib/targetAuthorisation";

/**
 * What one target holds, in a line.
 *
 * A target that is not connected holds whatever it was last written, but
 * saying so alongside "not connected" would be two answers to one question. The
 * connection is the answer that matters, so it is the one the line gives.
 */
function stagesSummary(target: TargetStatus): string {
  const authorisation = authorisationGuidance(target.authorisation);
  if (authorisation) {
    return authorisation.label;
  }

  const held =
    target.routes.pending === 0
      ? `All ${target.routes.current} ${target.routes.current === 1 ? "route" : "routes"}`
      : `${target.routes.current} of ${target.routes.current + target.routes.pending} routes`;
  const written = target.lastRun ? `written ${formatTimestamp(target.lastRun.completedAt)}` : null;

  return written ? `${held} · ${written}` : held;
}

/**
 * How this target's last write ended, when it did not end well.
 *
 * The service reduces every unsuccessful run to `failed` in its own one word,
 * because that word answers a different question — whether this target is
 * behind — and a held gate leaves it behind either way. Here there is room to
 * say which, and a gate must not be read as a fault: the target is intact and
 * the next move is the operator's.
 */
function lastRunSummary(target: TargetStatus): string | null {
  const guidance = targetGuidance(target);
  if (!guidance || !target.lastRun) {
    return null;
  }

  return `${GUIDANCE_LABELS[guidance.kind]} · ${formatTimestamp(target.lastRun.completedAt)}`;
}

/** One target's last write, explained, or nothing when it succeeded. */
function targetGuidance(target: TargetStatus) {
  return target.lastRun
    ? syncGuidance("targets", target.lastRun.result, target.lastRun.failure)
    : undefined;
}

export interface TargetRowProps {
  target: TargetStatus;
  /** Replaces reconcile and clear, for a page that offers something else. */
  actions?: ReactNode;
  reconciling?: boolean;
  onReconcile?: () => void;
  /**
   * The delete-confirmation dialog's state, held by the caller rather than in
   * this row: only one target's confirmation may be open at a time, which a
   * row cannot promise on its own.
   */
  clear?: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    confirmation: string;
    onConfirmationChange: (value: string) => void;
    pending: boolean;
    onConfirm: () => void;
  };
}

/** One target: what it holds, and the ways to act on it. */
export function TargetRow({ target, actions, reconciling, onReconcile, clear }: TargetRowProps) {
  const guidance = targetGuidance(target);
  const authorisation = authorisationGuidance(target.authorisation);
  const failure = lastRunSummary(target);
  // Clearing spends the shared deletion gate on a target that may not be this
  // caller's own in the admin fleet view, so it stays admin-only regardless
  // of which page renders this row.
  const effectiveAdmin = useEffectiveAdmin();

  const tone: RowTone = authorisation
    ? "hold"
    : guidance
      ? guidance.kind === "blocked"
        ? "hold"
        : "alert"
      : target.routes.pending === 0
        ? "good"
        : "hold";
  const note = authorisation ? (
    <RowNote tone="hold" data-kind="blocked">
      {authorisation.detail}
    </RowNote>
  ) : guidance ? (
    <RowNote tone={tone} data-kind={guidance.kind}>
      <strong>{guidance.headline}</strong> {guidance.remediation}
    </RowNote>
  ) : null;

  return (
    <InsetRow
      data-convergence={target.convergence}
      data-run={guidance?.kind}
      tone={tone}
      toneLabel={
        authorisation?.label ??
        (guidance ? GUIDANCE_LABELS[guidance.kind] : tone === "good" ? "In sync" : "Behind")
      }
      title={target.id}
      detail={[
        // Present only for an admin: a non-admin's own target is already known to be theirs.
        target.owner
          ? `Owned by ${target.owner}${target.ownerNickname ? ` (${target.ownerNickname})` : ""}`
          : null,
        stagesSummary(target),
        failure,
      ]}
      actions={
        authorisation?.action ? (
          <div className="flex shrink-0 items-center gap-2">
            {/*
             * A plain anchor, and a full-page navigation: the flow leaves this
             * application for Wahoo and returns to it, so there is nothing here
             * for a client-side route or a background request to carry. It is
             * a GET the service gates on the same identity as this page.
             */}
            <a
              className="inline-flex h-8 items-center rounded-[9px] bg-[var(--primary)] px-3 text-sm font-semibold text-[var(--primary-foreground)] hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
              href={authorisationStartHref(target.id)}
            >
              {authorisation.action} {target.id}
            </a>
          </div>
        ) : authorisation ? null : actions !== undefined ? (
          actions
        ) : (
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            {/*
             * While a deletion is being confirmed it is the only thing this
             * row offers. Leaving the ordinary action beside it would crowd
             * the sentence that explains what is about to happen, and put a
             * button next to it that undoes the point of asking.
             */}
            <Button
              variant="outline"
              disabled={reconciling}
              onClick={onReconcile}
              aria-label={`Reconcile now: ${target.id}`}
              icon={reconciling ? null : <IconRefresh />}
            >
              {reconciling ? <Spinner aria-label="Reconciling" /> : null}
            </Button>
            {/*
             * Deleting everything is not a variant of reconciling, so it does
             * not sit beside it as an equal. It asks first, and what it asks
             * for is the target's own name — the one confirmation a stray
             * click cannot supply.
             */}
            {effectiveAdmin && clear ? (
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
                    <AlertDialogCancel render={<Button variant="outline" />}>
                      Cancel
                    </AlertDialogCancel>
                    <Button
                      variant="destructive"
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
            ) : null}
          </div>
        )
      }
      note={note}
    />
  );
}
