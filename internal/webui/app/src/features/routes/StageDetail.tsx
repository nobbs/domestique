/**
 * One stage's full preview: the map, and the facts stored alongside it.
 *
 * Read-only by design. There are deliberately no editing affordances here, and
 * the service writes nothing back to VeloPlanner.
 */

import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError } from "../../api/client";
import { stageGeometryQuery, webUIConfigQuery } from "../../api/queries";
import type { Position, StageSurface } from "../../api/types";
import { ElevationProfile } from "../../components/ElevationProfile";
import { Layout } from "../../components/Layout";
import { ErrorMessage, LoadingMessage, StatusMessage } from "../../components/StatusMessage";
import { SurfaceBar } from "../../components/SurfaceBar";
import { formatAscent, formatDistance, formatGradient } from "../../lib/format";
import type { Profile } from "../../lib/profile";
import { buildProfile } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import { summariseSurface } from "../../lib/surface";
import { ReprocessButton } from "./ReprocessButton";

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

  const { stage, coordinates, bbox, surface } = geometry.data;

  return (
    <StageView
      stage={stage}
      routeId={routeId}
      coordinates={coordinates}
      bbox={bbox}
      surface={surface}
      styleUrl={config.data.tileStyleUrl}
      back={back}
    />
  );
}

/**
 * The map and the profile over one shared position.
 *
 * The hovered position lives here rather than in either view, because the two
 * are one instrument: pointing at the route marks the chart, and scrubbing the
 * chart marks the route. It is an index into the profile samples, so both sides
 * mean the same place by it.
 */
function StageView({
  stage,
  routeId,
  coordinates,
  bbox,
  surface,
  styleUrl,
  back,
}: {
  stage: {
    title: string;
    stageOrder: number;
    distanceMetres: number;
    ascentMetres: number;
    maxGradientPercent: number;
  };
  routeId: number;
  coordinates: Position[];
  bbox: [number, number, number, number];
  surface: StageSurface | undefined;
  styleUrl: string;
  back: React.ReactNode;
}) {
  const profile = useMemo(() => buildProfile(coordinates), [coordinates]);
  const [activeIndex, setActiveIndex] = useState<number | null>(null);

  // A classification that snapped to nothing is left unpainted rather than drawn
  // as unsurveyed from end to end: greying out the whole route to say nothing is
  // known says it less clearly than one sentence does.
  const surfaceSummary = useMemo(
    () =>
      surface && surface.matchedMetres > 0 ? summariseSurface(coordinates, surface.ranges) : null,
    [coordinates, surface],
  );

  return (
    <Layout status={back}>
      <section className="stage-detail">
        <header className="stage-detail__header">
          <div className="stage-detail__heading">
            <h1 className="stage-detail__title">{stage.title}</h1>
            <ReprocessButton routeId={routeId} stageOrder={stage.stageOrder} />
          </div>
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
              styleUrl={styleUrl}
              coordinates={coordinates}
              bbox={bbox}
              title={stage.title}
              surface={surfaceSummary ? surface?.ranges : undefined}
              profile={profile}
              activeIndex={activeIndex}
              onActiveChange={setActiveIndex}
            />
          </Suspense>
        </div>
        <ElevationOverview
          profile={profile}
          title={stage.title}
          surface={surfaceSummary}
          surfaceAbsence={
            surface
              ? "No OpenStreetMap surface data along this stage."
              : "Surface not classified yet."
          }
          activeIndex={activeIndex}
          onActiveChange={setActiveIndex}
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
  profile,
  title,
  surface,
  surfaceAbsence,
  hint,
  activeIndex,
  onActiveChange,
}: {
  profile: Profile | null;
  title: string;
  surface: SurfaceSummary | null;
  surfaceAbsence: string;
  hint: string;
  activeIndex: number | null;
  onActiveChange: (index: number | null) => void;
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
      {open ? (
        <>
          <ElevationProfile
            profile={profile}
            title={title}
            surface={surface}
            activeIndex={activeIndex}
            onActiveChange={onActiveChange}
          />
          {/*
           * The key sits under the strip it explains, not up beside the title:
           * a legend away from its marks is a lookup, and this one is meant to
           * be read in the same glance as the ground it names.
           */}
          {surface ? (
            <SurfaceBar summary={surface} />
          ) : (
            <p className="stage-detail__surface-absent">{surfaceAbsence}</p>
          )}
        </>
      ) : null}
    </details>
  );
}
