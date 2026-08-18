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
import { stageGeometryQuery, statusQuery } from "../../api/queries";

export function ReprocessButton({ routeId, stageOrder }: { routeId: number; stageOrder: number }) {
  const queryClient = useQueryClient();
  const reprocess = useMutation({
    mutationFn: () => reprocessStage(routeId, stageOrder),
    onSuccess: async () => {
      // The run happens in the background, so nothing here is fresh yet. Both
      // queries are marked stale so the page picks up the new geometry and the
      // new run result as they land rather than showing the old ones as though
      // the request had done nothing.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
        queryClient.invalidateQueries({
          queryKey: stageGeometryQuery(routeId, stageOrder).queryKey,
        }),
      ]);
    },
  });

  return (
    <div className="stage-detail__reprocess">
      <button
        type="button"
        className="stage-detail__reprocess-button"
        disabled={reprocess.isPending}
        onClick={() => reprocess.mutate()}
      >
        {reprocess.isPending ? "Requesting…" : "Reprocess"}
      </button>
      {reprocess.isSuccess ? (
        <span className="stage-detail__reprocess-note" role="status">
          Queued. This stage is read, derived, and pushed again on the next pass.
        </span>
      ) : null}
      {reprocess.isError ? (
        <span className="stage-detail__reprocess-note" role="status">
          {reprocess.error instanceof Error && reprocess.error.message
            ? reprocess.error.message
            : "That request could not be made."}
        </span>
      ) : null}
    </div>
  );
}
