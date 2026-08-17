/**
 * One stage on the map, with its stored facts. Read-only by design: there are
 * deliberately no editing affordances here, and the service writes nothing back
 * to VeloPlanner.
 */

import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense } from "react";
import { useParams } from "react-router";
import { ApiError } from "../../api/client";
import { stageGeometryQuery, webUIConfigQuery } from "../../api/queries";
import { ErrorMessage, LoadingMessage, StatusMessage } from "../../components/StatusMessage";
import { formatCount, formatDistance } from "../../lib/format";

// MapLibre is by far the heaviest dependency and only this view needs it, so it
// is fetched when a stage is opened rather than on first paint of the library.
const RouteMap = lazy(async () => ({
  default: (await import("../../components/RouteMap")).RouteMap,
}));

function positiveInteger(value: string | undefined): number | undefined {
  if (value === undefined || !/^\d+$/.test(value)) {
    return undefined;
  }
  const parsed = Number.parseInt(value, 10);
  return parsed > 0 ? parsed : undefined;
}

export function StageDetail() {
  const params = useParams();
  const routeId = positiveInteger(params.routeId);
  const stageOrder = positiveInteger(params.stage);
  const enabled = routeId !== undefined && stageOrder !== undefined;

  const config = useQuery(webUIConfigQuery());
  const geometry = useQuery({
    ...stageGeometryQuery(routeId ?? 0, stageOrder ?? 0),
    enabled,
  });

  if (!enabled) {
    return <StatusMessage tone="error" title="That is not a valid stage address." />;
  }
  if (geometry.isError && geometry.error instanceof ApiError && geometry.error.isNotFound) {
    return (
      <StatusMessage
        title="No geometry for this stage yet."
        detail="It appears after the next successful sync stores it."
      />
    );
  }
  if (geometry.isError) {
    return <ErrorMessage what="the route geometry" error={geometry.error} />;
  }
  if (config.isError) {
    return <ErrorMessage what="the map configuration" error={config.error} />;
  }
  if (geometry.isPending || config.isPending) {
    return <LoadingMessage what="the route" />;
  }

  const { stage, coordinates, bbox } = geometry.data;

  return (
    <section className="stage-detail">
      <header className="stage-detail__header">
        <h1 className="stage-detail__title">{stage.title}</h1>
        <dl className="stage-detail__facts">
          <div>
            <dt>Distance</dt>
            <dd>{formatDistance(stage.distanceMetres)}</dd>
          </div>
          <div>
            <dt>Points</dt>
            <dd>{formatCount(stage.pointCount, "point")}</dd>
          </div>
          <div>
            <dt>Stage</dt>
            <dd>{stage.stageOrder}</dd>
          </div>
        </dl>
      </header>
      <div className="stage-detail__map">
        <Suspense fallback={<LoadingMessage what="the map" />}>
          <RouteMap
            styleUrl={config.data.tileStyleUrl}
            coordinates={coordinates}
            bbox={bbox}
            title={stage.title}
          />
        </Suspense>
      </div>
    </section>
  );
}
