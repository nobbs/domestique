/**
 * A library grid card, with the data fetching a card needs.
 *
 * Each card loads its own geometry from the dedicated endpoint rather than the
 * listing carrying geometry, which keeps the rule that geometry is served only
 * by its own endpoint. It also warms the cache, so opening the route afterwards
 * renders immediately.
 *
 * The basemap is built only once a card nears the viewport, and the map library
 * itself is fetched on demand, so the library page stays cheap to open and a
 * long list does not exhaust the browser's WebGL contexts.
 */

import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense } from "react";
import { stageGeometryQuery, webUIConfigQuery } from "../../api/queries";
import type { Stage } from "../../api/types";
import { RouteCard } from "../../components/RouteCard";
import { RouteThumbnail } from "../../components/RouteThumbnail";
import { basemapFor, usePrefersDarkScheme } from "../../lib/basemap";
import { useInView } from "../../lib/useInView";

const RouteMiniMap = lazy(async () => ({
  default: (await import("../../components/RouteMiniMap")).RouteMiniMap,
}));

export function StageCard({ stage }: { stage: Stage }) {
  const { ref, inView } = useInView<HTMLLIElement>();
  const geometry = useQuery(stageGeometryQuery(stage.routeId, stage.stageOrder));
  const config = useQuery(webUIConfigQuery());
  const prefersDark = usePrefersDarkScheme();

  let preview: React.ReactNode;
  if (!geometry.data) {
    preview = (
      <span
        className="route-thumbnail route-thumbnail--empty"
        data-state={geometry.isError ? "error" : "loading"}
        aria-hidden="true"
      />
    );
  } else {
    const shape = <RouteThumbnail coordinates={geometry.data.coordinates} title={stage.title} />;
    preview =
      inView && config.data ? (
        <Suspense fallback={shape}>
          <RouteMiniMap
            styleUrl={basemapFor(config.data, prefersDark).styleUrl}
            coordinates={geometry.data.coordinates}
            bbox={geometry.data.bbox}
            title={stage.title}
          />
        </Suspense>
      ) : (
        shape
      );
  }

  return (
    <li ref={ref}>
      <RouteCard
        stage={stage}
        href={`/routes/${stage.routeId}/${stage.stageOrder}`}
        preview={preview}
      />
    </li>
  );
}
