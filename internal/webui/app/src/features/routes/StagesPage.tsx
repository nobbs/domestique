/**
 * The route library: a grid of stage cards, each linking to its full preview.
 * A later feature adds a sibling directory here rather than changing this one.
 */

import { useQuery } from "@tanstack/react-query";
import { stagesQuery } from "../../api/queries";
import { stageKey } from "../../api/types";
import { Layout } from "../../components/Layout";
import { RouteGrid } from "../../components/RouteCard";
import { ErrorMessage, LoadingMessage, StatusMessage } from "../../components/StatusMessage";
import { SyncControls } from "../sync/SyncControls";
import { TargetConvergence } from "../sync/TargetConvergence";
import { MapAttribution } from "./MapAttribution";
import { StageCard } from "./StageCard";
import { SyncStatusBadge } from "./SyncStatusBadge";

export function StagesPage() {
  const { data, isPending, isError, error } = useQuery(stagesQuery());

  return (
    <Layout status={<SyncStatusBadge />}>
      <SyncControls />
      <TargetConvergence />
      {isPending ? <LoadingMessage what="the route library" /> : null}
      {isError ? <ErrorMessage what="the route library" error={error} /> : null}
      {data && data.length === 0 ? (
        <StatusMessage
          title="No routes yet"
          detail="Stages appear here after the first successful synchronisation."
        />
      ) : null}
      {data && data.length > 0 ? (
        <>
          <RouteGrid>
            {data.map((stage) => (
              <StageCard key={stageKey(stage)} stage={stage} />
            ))}
          </RouteGrid>
          <MapAttribution />
        </>
      ) : null}
    </Layout>
  );
}
