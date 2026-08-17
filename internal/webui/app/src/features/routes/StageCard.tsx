/**
 * A library grid card, with the data fetching a card needs.
 *
 * Each card loads its own geometry from the dedicated endpoint rather than the
 * listing carrying geometry, which keeps the rule that geometry is served only
 * by its own endpoint. It also warms the cache, so opening the route afterwards
 * renders immediately.
 */

import { useQuery } from "@tanstack/react-query";
import { stageGeometryQuery } from "../../api/queries";
import type { Stage } from "../../api/types";
import { RouteCard } from "../../components/RouteCard";
import { RouteThumbnail } from "../../components/RouteThumbnail";

export function StageCard({ stage }: { stage: Stage }) {
  const geometry = useQuery(stageGeometryQuery(stage.routeId, stage.stageOrder));

  const preview = geometry.data ? (
    <RouteThumbnail coordinates={geometry.data.coordinates} title={stage.title} />
  ) : (
    <span
      className="route-thumbnail route-thumbnail--empty"
      data-state={geometry.isError ? "error" : "loading"}
      aria-hidden="true"
    />
  );

  return (
    <li>
      <RouteCard
        stage={stage}
        href={`/routes/${stage.routeId}/${stage.stageOrder}`}
        preview={preview}
      />
    </li>
  );
}
