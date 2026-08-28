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

import { useQueryClient } from "@tanstack/react-query";
import { useReprocessRoute } from "../../api/generated";
import { routeGeometryQuery, statusQuery } from "../../api/queries";
import { Button } from "../../components/Button";
import { Spinner } from "../../components/ui/spinner";

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
  const reprocess = useReprocessRoute({
    mutation: {
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
    },
  });

  return (
    <div className="grid gap-1">
      <Button
        variant="outline"
        disabled={reprocess.isPending}
        onClick={() => reprocess.mutate({ provider, routeId, stage: stageOrder })}
      >
        {reprocess.isPending ? <Spinner aria-label="Requesting reprocess" /> : null}
        {reprocess.isPending ? "Requesting…" : "Reprocess"}
      </Button>
      {reprocess.isSuccess ? (
        <span className="text-xs text-[var(--ink-2)]" role="status">
          Queued. This stage is read, derived, and pushed again on the next pass.
        </span>
      ) : null}
      {reprocess.isError ? (
        <span className="text-xs text-[var(--alert)]" role="status">
          {reprocess.error instanceof Error && reprocess.error.message
            ? reprocess.error.message
            : "That request could not be made."}
        </span>
      ) : null}
    </div>
  );
}
