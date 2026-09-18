/**
 * The rider's own Wahoo account, on their own settings page.
 *
 * `/v1/status` is already scoped to the caller: a non-admin sees only their
 * own target, so the first entry is theirs. An admin sees every target in slot
 * order, and only the server can tell which is theirs: it marks that one `own`.
 */

import { IconBike } from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Panel } from "@/components/PanelHeading";
import { useDisconnectWahoo } from "../../api/generated";
import { statusQuery, webUIConfigQuery } from "../../api/queries";
import type { TargetStatus } from "../../api/types";
import { Button } from "../../components/Button";
import { InsetList } from "../../components/InsetList";
import { Skeleton } from "../../components/ui/skeleton";
import { Spinner } from "../../components/ui/spinner";
import { ConnectPrompt } from "../sync/TargetConvergenceCard";
import { TargetRow } from "../sync/TargetRow";

function ownTarget(targets: TargetStatus[], admin: boolean): TargetStatus | undefined {
  if (!admin) {
    return targets[0];
  }

  return targets.find((target) => target.own);
}

function CardShell({ children }: { children: ReactNode }) {
  return (
    <Panel icon={<IconBike size={18} stroke={1.8} />} title="Wahoo account">
      <div className="grid gap-3">{children}</div>
    </Panel>
  );
}

export function WahooAccountCard() {
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(statusQuery());
  const {
    data: config,
    isPending: configIsPending,
    isError: configIsError,
  } = useQuery(webUIConfigQuery());
  const disconnect = useDisconnectWahoo({
    mutation: {
      onSettled: () => queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
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

  const target = ownTarget(data.targets, config.identity.admin);
  if (!target) {
    return (
      <CardShell>
        <ConnectPrompt />
      </CardShell>
    );
  }

  return (
    <CardShell>
      <InsetList>
        <TargetRow
          target={target}
          actions={
            <Button
              variant="outline"
              aria-label="Disconnect Wahoo account"
              disabled={disconnect.isPending}
              onClick={() => disconnect.mutate()}
            >
              {disconnect.isPending ? <Spinner aria-label="Disconnecting" /> : null}
              Disconnect
            </Button>
          }
        />
      </InsetList>
      {disconnect.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          Your Wahoo account was not disconnected.
        </p>
      ) : null}
    </CardShell>
  );
}
