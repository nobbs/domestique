/**
 * Asks for one stage to be worked out again from scratch.
 *
 * The service keeps three answers per stage — the derived geometry, the pushed
 * revision on each target, and the surface classification — and skips the work
 * whenever they still look current. This is the operator saying that one of them
 * is wrong: it drops all three for this stage and starts a synchronisation that
 * has to produce them again.
 *
 * It rewrites the route the service already owns on each target. It never
 * deletes one, never creates a second, and changes nothing in VeloPlanner.
 */

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { reprocessStage } from "../../api/client";
import { routeGeometryQuery, statusQuery } from "../../api/queries";
import { Button } from "../../components/Button";

export function ReprocessButton({
  provider,
  routeId,
  stageOrder,
}: {
  provider: string;
  routeId: number;
  stageOrder: number;
}) {
  const queryClient = useQueryClient();
  const reprocess = useMutation({
    mutationFn: () => reprocessStage(provider, routeId, stageOrder),
    onSuccess: async () => {
      await Promise.all([
        // The status is polled anyway, and refetching it now is how the run this
        // request started shows up as running rather than as nothing happening.
        queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
        // The geometry is only marked stale. Refetching it now would fetch the
        // old stage — the run has barely started — and then hold that answer as
        // fresh for the five minutes the query is allowed to cache, which is
        // exactly the wrong thing to do to a page waiting for new data. Stale
        // instead means the next visit or focus fetches whatever has landed by
        // then.
        queryClient.invalidateQueries({
          queryKey: routeGeometryQuery(provider, routeId, stageOrder).queryKey,
          refetchType: "none",
        }),
      ]);
    },
  });

  return (
    <div className="route-panel__reprocess">
      <Button variant="standard" disabled={reprocess.isPending} onClick={() => reprocess.mutate()}>
        {reprocess.isPending ? "Requesting…" : "Reprocess"}
      </Button>
      {reprocess.isSuccess ? (
        <span className="route-panel__reprocess-note" role="status">
          Queued. This stage is read, derived, and pushed again on the next pass.
        </span>
      ) : null}
      {reprocess.isError ? (
        <span className="route-panel__reprocess-note" role="status">
          {reprocess.error instanceof Error && reprocess.error.message
            ? reprocess.error.message
            : "That request could not be made."}
        </span>
      ) : null}
    </div>
  );
}
