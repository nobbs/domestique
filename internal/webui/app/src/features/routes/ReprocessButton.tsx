/**
 * Asks for one stage to be worked out again from scratch.
 *
 * The service keeps three answers per stage — the derived geometry, the pushed
 * revision on each target, and the surface classification — and skips the work
 * whenever they still look current. This is the operator saying that one of them
 * is wrong: it drops all three for this stage and starts a sync that
 * has to produce them again.
 *
 * It rewrites the route the service already owns on each target. It never
 * deletes one, never creates a second, and changes nothing in VeloPlanner.
 */

import { IconRefresh } from "@tabler/icons-react";
import { useQueryClient } from "@tanstack/react-query";
import { useReprocessRoute } from "../../api/generated";
import { routeGeometryQuery, statusQuery } from "../../api/queries";
import { DropdownMenuItem } from "../../components/ui/dropdown-menu";
import { Spinner } from "../../components/ui/spinner";

export function ReprocessButton({
  provider,
  sourceRouteId,
  stageOrder,
}: {
  provider: string;
  sourceRouteId: number;
  stageOrder: number;
}) {
  const queryClient = useQueryClient();
  const reprocess = useReprocessRoute({
    mutation: {
      onSuccess: async () => {
        await Promise.all([
          // The status is polled anyway, and refetching it now is how the run this
          // request started shows up as running rather than as nothing happening.
          queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
          // The geometry is only marked stale. Refetching now would fetch the old stage
          // — the run has barely started — and hold it as fresh for the five minutes the
          // query caches. Stale means the next visit or focus fetches what has landed.
          queryClient.invalidateQueries({
            queryKey: routeGeometryQuery(provider, sourceRouteId, stageOrder).queryKey,
            refetchType: "none",
          }),
        ]);
      },
    },
  });

  return (
    <>
      <DropdownMenuItem
        // The only item here that does not close the menu. What it asks for has
        // an answer — queued, or refused with a reason — and the menu is where
        // the reader is looking when it arrives. Closing on click would put the
        // outcome nowhere.
        closeOnClick={false}
        disabled={reprocess.isPending}
        onClick={() => reprocess.mutate({ provider, sourceRouteId, stageOrder })}
      >
        {reprocess.isPending ? (
          <Spinner aria-label="Requesting reprocess" />
        ) : (
          <IconRefresh aria-hidden="true" />
        )}
        {reprocess.isPending ? "Requesting\u2026" : "Reprocess"}
      </DropdownMenuItem>
      {reprocess.isSuccess ? (
        <p className="px-1.5 pb-1 text-xs text-[var(--ink-2)]" role="status">
          Queued. This route is read, derived, and pushed again on the next pass.
        </p>
      ) : null}
      {reprocess.isError ? (
        <p className="px-1.5 pb-1 text-xs text-[var(--alert)]" role="status">
          {reprocess.error instanceof Error && reprocess.error.message
            ? reprocess.error.message
            : "That request could not be made."}
        </p>
      ) : null}
    </>
  );
}
