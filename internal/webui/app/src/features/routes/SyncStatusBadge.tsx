/**
 * A compact readout of the last sync, so the operator can tell how fresh the
 * map is. The map only ever shows what the last successful run stored.
 */

import { useQuery } from "@tanstack/react-query";
import { statusQuery } from "../../api/queries";
import { formatTimestamp } from "../../lib/format";

export function SyncStatusBadge() {
  const { data, isPending, isError } = useQuery(statusQuery());

  if (isPending || isError || !data) {
    return null;
  }

  return (
    <span className="sync-badge" data-state={data.sync.state}>
      <span className="sync-badge__dot" aria-hidden="true" />
      <span>
        {data.sync.lastResult ?? data.sync.state} · synced{" "}
        {formatTimestamp(data.sync.lastCompletedAt)}
      </span>
    </span>
  );
}
