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

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useClearTarget, useTriggerTargetSync } from "../../api/generated";
import { statusQuery } from "../../api/queries";
import { Skeleton } from "../../components/ui/skeleton";
import { formatCount, formatTimestamp } from "../../lib/format";
import { AccountRow } from "./AccountRow";

/** The body of the "What the accounts hold" card: one row per account. */
export function TargetConvergence() {
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(statusQuery());
  const reconcile = useTriggerTargetSync({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
    },
  });
  // Which account is being cleared, and what has been typed to confirm it.
  // Holding the slot here rather than per row is what keeps two confirmations
  // from ever being open at once.
  const [clearing, setClearing] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState("");
  const clear = useClearTarget({
    mutation: {
      onSuccess: () => {
        setClearing(null);
        setConfirmation("");

        return queryClient.invalidateQueries({ queryKey: statusQuery().queryKey });
      },
    },
  });

  if (isPending) {
    return <Skeleton className="h-24 w-full" role="status" aria-label="Loading accounts" />;
  }
  if (isError) {
    return (
      <p className="text-sm text-[var(--alert)]">The service did not say what the accounts hold.</p>
    );
  }

  return (
    <>
      <ul className="grid gap-3">
        {data.targets.map((target) => (
          <AccountRow
            key={target.id}
            target={target}
            reconciling={reconcile.isPending}
            onReconcile={() => reconcile.mutate({ target: target.id })}
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
              onConfirm: () => clear.mutate({ target: target.id }),
            }}
          />
        ))}
      </ul>
      <p className="text-sm text-[var(--ink-2)]">
        This is what the accounts hold, not what a head unit has downloaded.
      </p>
      {/*
       * Wahoo's own live reading, not a count this service keeps itself: the
       * quota is shared across every configured account, so nothing local
       * could total it correctly on its own. Absent until a request has
       * actually reached Wahoo and reported one.
       */}
      {data.sync.wahooRateLimit ? (
        <p className="text-sm text-[var(--ink-2)]">
          Wahoo has {formatCount(data.sync.wahooRateLimit.remaining, "request")} left, shared by
          every account here.
          {data.sync.wahooRateLimit.resetsAt
            ? ` Resets ${formatTimestamp(data.sync.wahooRateLimit.resetsAt)}.`
            : null}
        </p>
      ) : null}
      {reconcile.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          {reconcile.error instanceof Error && reconcile.error.message
            ? reconcile.error.message
            : "That account could not be reconciled."}
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
