/**
 * What each target holds.
 *
 * This is the question an operator actually has after a sync: not "did the run
 * succeed" but "is what I planned now on both targets". The two are different —
 * a run that wrote one target and could not write the other is recorded once,
 * as failed, and says nothing about which target is behind.
 *
 * It is deliberately a claim about the Wahoo account behind the target and
 * nothing further:
 * whether a head unit has since downloaded those routes is not something the
 * service can see, so the card says so rather than letting "up to date" be read
 * as "on the device".
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useRunTaskArgument } from "../../api/generated";
import { statusQuery, webUIConfigQuery } from "../../api/queries";
import { TASKS } from "../../api/tasks";
import { Skeleton } from "../../components/ui/skeleton";
import { formatCount, formatTimestamp } from "../../lib/format";
import { TargetRow } from "./TargetRow";

/**
 * A rider with no target yet: the one way to get one is to connect their own
 * Wahoo account, and the flow needs no target identifier to start — the
 * browser is never told its own subject.
 */
function ConnectPrompt() {
  return (
    <p className="text-sm text-[var(--ink-2)]">
      Your Wahoo account is not connected yet.{" "}
      <a
        className="font-semibold text-[var(--accent)] underline-offset-4 hover:underline"
        href="/oauth/wahoo/start"
      >
        Connect it
      </a>{" "}
      to start writing routes to it.
    </p>
  );
}

/** The body of the "What the targets hold" card: one row per target. */
export function TargetConvergenceCard() {
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(statusQuery());
  const { data: config } = useQuery(webUIConfigQuery());
  const reconcile = useRunTaskArgument({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
    },
  });
  // Which target is being cleared, and what has been typed to confirm it.
  // Holding the slot here rather than per row is what keeps two confirmations
  // from ever being open at once.
  const [clearing, setClearing] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState("");
  const clear = useRunTaskArgument({
    mutation: {
      onSuccess: () => {
        setClearing(null);
        setConfirmation("");

        return queryClient.invalidateQueries({ queryKey: statusQuery().queryKey });
      },
    },
  });

  if (isPending) {
    return <Skeleton className="h-24 w-full" role="status" aria-label="Loading targets" />;
  }
  if (isError) {
    return (
      <p className="text-sm text-[var(--alert)]">The service did not say what the targets hold.</p>
    );
  }
  // Nothing to reconcile before there is a target, and no target before its
  // owner connects — an admin sees every rider's, so an empty list here means
  // none exist yet rather than that this caller's own is missing.
  if (data.targets.length === 0) {
    return config?.identity.admin ? (
      <p className="text-sm text-[var(--ink-2)]">No target has connected yet.</p>
    ) : (
      <ConnectPrompt />
    );
  }

  return (
    <>
      <ul className="grid gap-3">
        {data.targets.map((target) => (
          <TargetRow
            key={target.id}
            target={target}
            reconciling={reconcile.isPending}
            onReconcile={() => reconcile.mutate({ name: TASKS.syncTarget, argument: target.id })}
            clear={{
              open: clearing === target.id,
              onOpenChange: (open) => {
                setClearing(open ? target.id : null);
                if (!open) {
                  setConfirmation("");
                }
              },
              confirmation,
              onConfirmationChange: setConfirmation,
              pending: clear.isPending,
              onConfirm: () => clear.mutate({ name: TASKS.syncClear, argument: target.id }),
            }}
          />
        ))}
      </ul>
      <p className="text-sm text-[var(--ink-2)]">
        This is what the targets hold, not what a head unit has downloaded.
      </p>
      {/*
       * Wahoo's own live reading, not a count this service keeps itself: the
       * quota is shared across every configured target, so nothing local
       * could total it correctly on its own. Absent until a request has
       * actually reached Wahoo and reported one.
       */}
      {data.sync.wahooRateLimit ? (
        <p className="text-sm text-[var(--ink-2)]">
          Wahoo has {formatCount(data.sync.wahooRateLimit.remaining, "request")} left, shared by
          every target here.
          {data.sync.wahooRateLimit.resetsAt
            ? ` Resets ${formatTimestamp(data.sync.wahooRateLimit.resetsAt)}.`
            : null}
        </p>
      ) : null}
      {reconcile.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          {reconcile.error instanceof Error && reconcile.error.message
            ? reconcile.error.message
            : "That target could not be reconciled."}
        </p>
      ) : null}
      {clear.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          {clear.error instanceof Error && clear.error.message
            ? clear.error.message
            : "Those routes could not be deleted."}
        </p>
      ) : null}
    </>
  );
}
