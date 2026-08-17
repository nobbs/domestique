/**
 * The route library page. It owns its own data fetching and composes the shared
 * layout, list, and map components. A later feature adds a sibling directory
 * here rather than changing this one.
 */

import { useQuery } from "@tanstack/react-query";
import { Outlet, useParams } from "react-router";
import { stagesQuery } from "../../api/queries";
import type { Stage } from "../../api/types";
import { Layout } from "../../components/Layout";
import { StageList } from "../../components/StageList";
import { ErrorMessage, LoadingMessage, StatusMessage } from "../../components/StatusMessage";
import { SyncStatusBadge } from "./SyncStatusBadge";

function hrefFor(stage: Stage): string {
  return `/routes/${stage.routeId}/${stage.stageOrder}`;
}

export function StagesPage() {
  const { data, isPending, isError, error } = useQuery(stagesQuery());
  const params = useParams();
  const hasSelection = params.routeId !== undefined;

  let sidebar: React.ReactNode;
  if (isPending) {
    sidebar = <LoadingMessage what="stages" />;
  } else if (isError) {
    sidebar = <ErrorMessage what="the stage list" error={error} />;
  } else {
    sidebar = <StageList stages={data} hrefFor={hrefFor} />;
  }

  return (
    <Layout sidebar={sidebar} status={<SyncStatusBadge />}>
      {hasSelection ? (
        <Outlet />
      ) : (
        <StatusMessage title="Select a stage" detail="Choose a route stage to see it on the map." />
      )}
    </Layout>
  );
}
