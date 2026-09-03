/**
 * The rider's own Wahoo account, on their own settings page.
 *
 * `/v1/status` is already scoped to the caller: a non-admin sees only their
 * own target, so the first entry is theirs. An admin sees every target, so
 * the one without an `owner` is the one connected under their own subject —
 * `TargetRow`'s owner line exists for exactly the opposite case, another
 * rider's target in the admin's fleet view.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useRunTaskArgument } from "../../api/generated";
import { statusQuery, webUIConfigQuery } from "../../api/queries";
import { TASKS } from "../../api/tasks";
import type { TargetStatus } from "../../api/types";
import { Skeleton } from "../../components/ui/skeleton";
import { useEffectiveAdmin } from "../../lib/identity";
import { ConnectPrompt } from "../sync/TargetConvergenceCard";
import { TargetRow } from "../sync/TargetRow";

function ownTarget(targets: TargetStatus[], effectiveAdmin: boolean): TargetStatus | undefined {
  if (!effectiveAdmin) {
    return targets[0];
  }

  return targets.find((target) => !target.owner) ?? targets[0];
}

function CardShell({ children }: { children: ReactNode }) {
  return (
    <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
      <CardHeader>
        <CardTitle role="heading" aria-level={2}>
          Wahoo account
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3">{children}</CardContent>
    </Card>
  );
}

export function WahooAccountCard() {
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(statusQuery());
  const { isPending: configIsPending, isError: configIsError } = useQuery(webUIConfigQuery());
  const effectiveAdmin = useEffectiveAdmin();
  const [confirming, setConfirming] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const reconcile = useRunTaskArgument({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
    },
  });
  const clear = useRunTaskArgument({
    mutation: {
      onSuccess: () => {
        setConfirming(false);
        setConfirmation("");

        return queryClient.invalidateQueries({ queryKey: statusQuery().queryKey });
      },
    },
  });

  if (isPending || configIsPending) {
    return (
      <CardShell>
        <Skeleton className="h-16 w-full" role="status" aria-label="Loading your Wahoo account" />
      </CardShell>
    );
  }
  if (isError || configIsError) {
    return (
      <CardShell>
        <p className="text-sm text-[var(--alert)]">
          The service did not say what your Wahoo account holds.
        </p>
      </CardShell>
    );
  }

  const target = ownTarget(data.targets, effectiveAdmin);
  if (!target) {
    return (
      <CardShell>
        <ConnectPrompt />
      </CardShell>
    );
  }

  return (
    <CardShell>
      <ul className="grid gap-3">
        <TargetRow
          target={target}
          reconciling={reconcile.isPending}
          onReconcile={() => reconcile.mutate({ name: TASKS.syncTarget, argument: target.id })}
          clear={{
            open: confirming,
            onOpenChange: (open) => {
              setConfirming(open);
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
      </ul>
    </CardShell>
  );
}
