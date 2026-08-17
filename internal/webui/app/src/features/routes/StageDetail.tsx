/**
 * One stage's full preview: the map, and the facts stored alongside it.
 *
 * Read-only by design. There are deliberately no editing affordances here, and
 * the service writes nothing back to VeloPlanner.
 */

import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError } from "../../api/client";
import { stageGeometryQuery, webUIConfigQuery } from "../../api/queries";
import type { Position } from "../../api/types";
import { ElevationProfile } from "../../components/ElevationProfile";
import { Layout } from "../../components/Layout";
import { ErrorMessage, LoadingMessage, StatusMessage } from "../../components/StatusMessage";
import { formatAscent, formatDistance, formatGradient } from "../../lib/format";

// MapLibre is by far the heaviest dependency and only this view needs it, so it
// is fetched when a route is opened rather than on first paint of the library.
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

  const back = (
    <Link to="/" className="stage-detail__back">
      ← All routes
    </Link>
  );

  if (!enabled) {
    return (
      <Layout status={back}>
        <StatusMessage tone="error" title="That is not a valid route address." />
      </Layout>
    );
  }
  if (geometry.isError && geometry.error instanceof ApiError && geometry.error.isNotFound) {
    return (
      <Layout status={back}>
        <StatusMessage
          title="No geometry for this route yet."
          detail="It appears after the next successful synchronisation stores it."
        />
      </Layout>
    );
  }
  if (geometry.isError) {
    return (
      <Layout status={back}>
        <ErrorMessage what="the route geometry" error={geometry.error} />
      </Layout>
    );
  }
  if (config.isError) {
    return (
      <Layout status={back}>
        <ErrorMessage what="the map configuration" error={config.error} />
      </Layout>
    );
  }
  if (geometry.isPending || config.isPending) {
    return (
      <Layout status={back}>
        <LoadingMessage what="the route" />
      </Layout>
    );
  }

  const { stage, coordinates, bbox } = geometry.data;

  return (
    <Layout status={back}>
      <section className="stage-detail">
        <header className="stage-detail__header">
          <h1 className="stage-detail__title">{stage.title}</h1>
          <dl className="stage-detail__facts">
            <div>
              <dt>Distance</dt>
              <dd>{formatDistance(stage.distanceMetres)}</dd>
            </div>
            <div>
              <dt>Ascent</dt>
              <dd>{formatAscent(stage.ascentMetres)}</dd>
            </div>
            <div>
              <dt>Max gradient</dt>
              <dd>{formatGradient(stage.maxGradientPercent)}</dd>
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
        <ElevationOverview
          coordinates={coordinates}
          title={stage.title}
          hint={`${formatAscent(stage.ascentMetres)} climbing · ${formatGradient(stage.maxGradientPercent)} max`}
        />
      </section>
    </Layout>
  );
}

const OVERVIEW_PREFERENCE = "domestique.elevation-overview-open";

/**
 * The profile beneath the map, collapsible so the map can have the whole pane.
 *
 * The choice is remembered, because it is a standing preference about how the
 * page is read rather than something to re-make on every route. The chart is
 * only mounted while open, so a collapsed overview costs no layout work.
 */
function ElevationOverview({
  coordinates,
  title,
  hint,
}: {
  coordinates: Position[];
  title: string;
  hint: string;
}) {
  const [open, setOpen] = useState(() => {
    try {
      return localStorage.getItem(OVERVIEW_PREFERENCE) !== "closed";
    } catch {
      // Storage can be unavailable; the overview simply opens by default.
      return true;
    }
  });

  const onToggle = (event: React.SyntheticEvent<HTMLDetailsElement>) => {
    const next = event.currentTarget.open;
    setOpen(next);
    try {
      localStorage.setItem(OVERVIEW_PREFERENCE, next ? "open" : "closed");
    } catch {
      // A preference that cannot be stored is not worth failing the page over.
    }
  };

  return (
    <details className="elevation-overview" open={open} onToggle={onToggle}>
      <summary className="elevation-overview__summary">
        <svg className="elevation-overview__caret" viewBox="0 0 12 12" aria-hidden="true">
          <path
            d="M4.5 2.5 L8 6 L4.5 9.5"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
        <span>Elevation</span>
        {/* Kept meaningful when closed, so collapsing does not hide the numbers. */}
        <span className="elevation-overview__hint">{hint}</span>
      </summary>
      {open ? <ElevationProfile coordinates={coordinates} title={title} /> : null}
    </details>
  );
}
