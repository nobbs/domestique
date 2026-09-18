import { Tabs } from "@base-ui/react/tabs";
import {
  IconArrowBackUp,
  IconArrowDownRight,
  IconArrowForwardUp,
  IconArrowsExchange,
  IconArrowUpRight,
  IconBan,
  IconClock,
  IconLayoutBottombarCollapse,
  IconMountain,
  IconRoad,
  IconWalk,
} from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useMemo, useReducer, useRef, useState } from "react";
import { Layer, Marker, ScaleControl, Source } from "react-map-gl/maplibre";
import { useLocation, useNavigate, useParams } from "react-router";
import {
  getGetPlanDeliveryQueryKey,
  getGetPlanQueryKey,
  getListPlansQueryKey,
  snapPlace,
  useCreatePlan,
  useDeletePlan,
  useGetPlan,
  usePreviewPlanRoute,
  useReplacePlan,
} from "../../api/generated";
import { webUIConfigQuery } from "../../api/queries";
import type {
  BoundingBox,
  Plan,
  PlanRoutePreview,
  PlanWindow,
  Position,
  SnappedPlace,
} from "../../api/types";
import { Button } from "../../components/Button";
import { Layout, PageShell } from "../../components/Layout";
import { BasemapPicker } from "../../components/map/BasemapPicker";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapControls } from "../../components/map/MapControls";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { SegmentedTrack, SegmentLabel, segmentClass } from "../../components/Segmented";
import { Alert, AlertDescription, AlertTitle } from "../../components/ui/alert";
import { ButtonGroup } from "../../components/ui/button-group";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import { ROUTE_MAX_ZOOM } from "../../lib/cartography";
import { formatAscent, formatDistance, formatDuration } from "../../lib/format";
import { useNarrowViewport } from "../../lib/mediaQuery";
import { groundSegments, steepnessEntries, surfaceEntries } from "../../lib/mix";
import {
  buildProfile,
  coordinateRange,
  gradientSharesBySign,
  rangeBounds,
} from "../../lib/profile";
import { boxAround, LOCATION_ZOOM, useStartupLocation } from "../../lib/startupLocation";
import { type SurfaceSummary, summariseSurface } from "../../lib/surface";
import { resolvesDark, useThemeChoice } from "../../lib/theme";
import { useEscapeKey } from "../../lib/useEscapeKey";
import { ElevationProfile } from "../routes/ElevationProfile";
import { GroundRibbon } from "../routes/GroundRibbon";
import { RouteOverlay } from "../routes/RouteOverlay";
import {
  type HiddenRun,
  PlannerSidebar,
  type PlannerSidebarProps,
  WaypointMarker,
  waypointLabel,
} from "./PlannerSidebar";
import {
  initialPlannerState,
  isPlannerSeed,
  type PlannerAvoid,
  type PlannerSeed,
  type PlannerState,
  plannerReducer,
  unwrapped,
} from "./planner";

function positions(preview: PlanRoutePreview | null): Position[] {
  return (preview?.geometry.coordinates ?? []).flatMap(([longitude, latitude, elevation]) => {
    if (longitude === undefined || latitude === undefined) {
      return [];
    }

    return [
      elevation === undefined ? [longitude, latitude] : [longitude, latitude, elevation],
    ] as Position[];
  });
}

function previewFrom(plan: Plan): PlanRoutePreview {
  return {
    geometry: plan.geometry,
    distanceMetres: plan.distanceMetres,
    ascentMetres: plan.ascentMetres,
    ...(plan.descentMetres === undefined ? {} : { descentMetres: plan.descentMetres }),
    ...(plan.movingSeconds === undefined ? {} : { movingSeconds: plan.movingSeconds }),
    ...(plan.waypointProgress === undefined ? {} : { waypointProgress: plan.waypointProgress }),
    ...(plan.pushing === undefined ? {} : { pushing: plan.pushing }),
    ...(plan.surface === undefined ? {} : { surface: plan.surface }),
  };
}

/** How far a plan runs along ways a rider walks, in metres. */
function pushingMetres(preview: PlanRoutePreview | null): number {
  return (preview?.pushing ?? []).reduce(
    (total, window) => total + window.endMetres - window.startMetres,
    0,
  );
}

/**
 * The stretches a rider walks, dashed over the route so they read as footpath
 * whatever colour the ground under them paints the line.
 */
function PushingLine({ line, pushing }: { line: Position[]; pushing: PlanWindow[] }) {
  const data = useMemo(() => {
    const stretches = pushing.flatMap((window) => {
      const range = coordinateRange(line, window.startMetres, window.endMetres);
      return range ? [line.slice(range.startIndex, range.endIndex + 1)] : [];
    });

    return {
      type: "Feature" as const,
      properties: {},
      geometry: { type: "MultiLineString" as const, coordinates: stretches },
    };
  }, [line, pushing]);

  return (
    <Source id="plan-pushing" type="geojson" data={data}>
      <Layer
        id="plan-pushing-line"
        type="line"
        layout={{ "line-cap": "butt", "line-join": "round" }}
        paint={{ "line-color": "#ffffff", "line-width": 3, "line-dasharray": [0.8, 1.6] }}
      />
    </Source>
  );
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Could not route these waypoints.";
}

function planWaypoints(waypoints: Plan["waypoints"]) {
  return waypoints.map(({ longitude, latitude, straight }) =>
    straight ? { longitude, latitude, straight } : { longitude, latitude },
  );
}

/** What a save sends, the publication aside; also what tells an edited plan from its stored one. */
function planBody(
  plan: Pick<Plan, "name" | "profile" | "cues" | "waypoints"> & { avoid?: Plan["avoid"] },
) {
  const avoid = plan.avoid ?? [];
  return {
    name: plan.name.trim(),
    profile: plan.profile,
    cues: plan.cues,
    waypoints: planWaypoints(plan.waypoints),
    ...(avoid.length === 0 ? {} : { avoid: planAvoid(avoid) }),
  };
}

function planAvoid(avoid: NonNullable<Plan["avoid"]>) {
  return avoid.map(({ longitude, latitude, radiusMetres }) => ({
    longitude,
    latitude,
    radiusMetres,
  }));
}

const EARTH_METRES_PER_DEGREE = 111_320;
const AVOID_CIRCLE_VERTICES = 64;

/** A polygon approximating a circle of `radiusMetres`, for a fill the router's own math never draws. */
function avoidCirclePolygon(area: PlannerAvoid): Position[] {
  const latitudeDelta = area.radiusMetres / EARTH_METRES_PER_DEGREE;
  const longitudeDelta =
    area.radiusMetres / (EARTH_METRES_PER_DEGREE * Math.cos((area.latitude * Math.PI) / 180));

  return Array.from({ length: AVOID_CIRCLE_VERTICES + 1 }, (_, index) => {
    const angle = (index / AVOID_CIRCLE_VERTICES) * 2 * Math.PI;

    return [
      area.longitude + longitudeDelta * Math.cos(angle),
      area.latitude + latitudeDelta * Math.sin(angle),
    ] as Position;
  });
}

/** MapLibre paint properties take literal colours, not CSS custom properties. */
function useThemeColour(property: string, fallback: string): string {
  return useMemo(() => {
    if (typeof document === "undefined") {
      return fallback;
    }
    const value = getComputedStyle(document.documentElement).getPropertyValue(property).trim();

    return value || fallback;
  }, [property, fallback]);
}

const AVOID_MIN_METRES = 10;
const AVOID_MAX_METRES = 5000;

/** The point on an avoided circle's eastern edge, where its resize handle sits. */
function avoidEdge(area: PlannerAvoid): { longitude: number; latitude: number } {
  return {
    longitude:
      area.longitude +
      area.radiusMetres / (EARTH_METRES_PER_DEGREE * Math.cos((area.latitude * Math.PI) / 180)),
    latitude: area.latitude,
  };
}

/** The radius a handle dragged to `edge` gives an area, clamped to what the service accepts. */
export function avoidRadiusTo(
  area: { longitude: number; latitude: number },
  edge: { longitude: number; latitude: number },
): number {
  const x =
    (edge.longitude - area.longitude) *
    EARTH_METRES_PER_DEGREE *
    Math.cos((area.latitude * Math.PI) / 180);
  const y = (edge.latitude - area.latitude) * EARTH_METRES_PER_DEGREE;

  return Math.round(Math.min(AVOID_MAX_METRES, Math.max(AVOID_MIN_METRES, Math.hypot(x, y))));
}

/** The stretch of line a folded run of waypoints covers, drawn over the route while its row is hovered. */
function HiddenRunLayer({
  line,
  preview,
  run,
}: {
  line: Position[];
  preview: PlanRoutePreview | null;
  run: HiddenRun | null;
}) {
  const colour = useThemeColour("--accent", "#2f6fdb");
  const progress = preview?.waypointProgress;
  const start = run ? progress?.[run.from - 1]?.distanceMetres : undefined;
  const end = run ? progress?.[run.to + 1]?.distanceMetres : undefined;
  const range = start === undefined || end === undefined ? null : coordinateRange(line, start, end);
  const data = {
    type: "FeatureCollection" as const,
    features: range
      ? [
          {
            type: "Feature" as const,
            properties: {},
            geometry: {
              type: "LineString" as const,
              coordinates: line.slice(range.startIndex, range.endIndex + 1),
            },
          },
        ]
      : [],
  };

  return (
    <Source id="plan-hidden-run" type="geojson" data={data}>
      <Layer
        id="plan-hidden-run-line"
        type="line"
        layout={{ "line-cap": "round", "line-join": "round" }}
        paint={{ "line-color": colour, "line-width": 7, "line-opacity": 0.9 }}
      />
    </Source>
  );
}

/** The circles a plan keeps its route out of, filled and outlined in the alert tone. */
function AvoidAreasLayer({ areas }: { areas: PlannerAvoid[] }) {
  const colour = useThemeColour("--alert", "#c0392b");
  const data = useMemo(
    () => ({
      type: "Feature" as const,
      properties: {},
      geometry: {
        type: "MultiPolygon" as const,
        coordinates: areas.map((area) => [avoidCirclePolygon(area)]),
      },
    }),
    [areas],
  );

  if (areas.length === 0) {
    return null;
  }

  return (
    <Source id="plan-avoid" type="geojson" data={data}>
      <Layer
        id="plan-avoid-fill"
        type="fill"
        paint={{ "fill-color": colour, "fill-opacity": 0.18 }}
      />
      <Layer
        id="plan-avoid-outline"
        type="line"
        paint={{ "line-color": colour, "line-width": 2 }}
      />
    </Source>
  );
}

function waypointPositions(waypoints: Array<{ longitude: number; latitude: number }>): Position[] {
  return waypoints.map(({ longitude, latitude }) => [longitude, latitude]);
}

/** The one-shot framing of an opened plan or seed: later edits never move the camera. */
function framing(planId: number | null, plan: Plan | PlannerSeed): PlannerFraming {
  const line = "geometry" in plan ? positions(previewFrom(plan)) : [];
  const coordinates = line.length > 1 ? line : waypointPositions(plan.waypoints);

  return {
    planId,
    bounds: rangeBounds(coordinates, { startIndex: 0, endIndex: coordinates.length - 1 }),
  };
}

interface PlannerFraming {
  planId: number | null;
  bounds: BoundingBox | null;
}

function insertionIndex(
  waypoints: PlannerState["waypoints"],
  waypoint: { longitude: number; latitude: number },
): number {
  if (waypoints.length < 2) {
    return waypoints.length;
  }
  let nearest = 0;
  let nearestDistance = Number.POSITIVE_INFINITY;
  let nearestIsFinalEndpoint = false;

  for (let index = 0; index < waypoints.length - 1; index++) {
    const start = waypoints[index];
    const end = waypoints[index + 1];
    if (!start || !end) {
      continue;
    }
    const longitudeScale = Math.max(
      0.01,
      Math.cos(((start.latitude + end.latitude + waypoint.latitude) / 3) * (Math.PI / 180)),
    );
    const endX = (end.longitude - start.longitude) * longitudeScale;
    const endY = end.latitude - start.latitude;
    const pointX = (waypoint.longitude - start.longitude) * longitudeScale;
    const pointY = waypoint.latitude - start.latitude;
    const lengthSquared = endX * endX + endY * endY;
    const projection = lengthSquared === 0 ? 0 : (pointX * endX + pointY * endY) / lengthSquared;
    const fraction = Math.max(0, Math.min(1, projection));
    const distance = (pointX - endX * fraction) ** 2 + (pointY - endY * fraction) ** 2;

    if (distance < nearestDistance) {
      nearest = index;
      nearestDistance = distance;
      nearestIsFinalEndpoint = index === waypoints.length - 2 && projection >= 1;
    }
  }

  return nearestIsFinalEndpoint ? waypoints.length : nearest + 1;
}

/** The plan's own figures, on the strip beneath the map where the chart is. */
function PlanFigures({ preview }: { preview: PlanRoutePreview | null }) {
  if (!preview) {
    return null;
  }
  const figure = (icon: ReactNode, value: string) => (
    <span className="flex items-baseline gap-1 text-[var(--ink-2)]">
      {icon}
      <span className="font-medium text-[var(--ink)] tabular-nums">{value}</span>
    </span>
  );

  return (
    <output aria-label="Planned route summary" className="flex items-center gap-4 text-sm">
      <span className="font-semibold text-base tabular-nums">
        {formatDistance(preview.distanceMetres)}
      </span>
      {figure(
        <IconArrowUpRight size={14} stroke={1.8} aria-hidden="true" />,
        formatAscent(preview.ascentMetres),
      )}
      {preview.descentMetres === undefined
        ? null
        : figure(
            <IconArrowDownRight size={14} stroke={1.8} aria-hidden="true" />,
            formatAscent(preview.descentMetres),
          )}
      {preview.movingSeconds === undefined
        ? null
        : figure(
            <IconClock size={14} stroke={1.8} aria-hidden="true" />,
            formatDuration(Math.round(preview.movingSeconds / 60) * 60),
          )}
      {pushingMetres(preview) > 0 ? (
        <span title="Walked, where bicycles are refused">
          {figure(
            <IconWalk size={14} stroke={1.8} aria-label="Walked" />,
            formatDistance(pushingMetres(preview)),
          )}
        </span>
      ) : null}
    </output>
  );
}

function PlannerHistoryControls({
  state,
  dispatch,
}: Pick<PlannerSidebarProps, "state" | "dispatch">) {
  return (
    <div className="pointer-events-auto absolute top-3 left-3 z-10 flex items-center gap-2">
      <ButtonGroup
        aria-label="Planner history"
        orientation="horizontal"
        className="divide-x divide-[var(--rule)] rounded-[9px] bg-[var(--panel)] shadow-[var(--shadow)] ring-1 ring-[var(--rule)] ring-inset [&>*:not(:first-child)]:rounded-l-none [&>*:not(:last-child)]:rounded-r-none"
      >
        <Button
          variant="ghost"
          icon={<IconArrowBackUp stroke={1.6} />}
          disabled={state.past.length === 0}
          aria-label="Undo"
          title="Undo"
          onClick={() => dispatch({ type: "undo" })}
        />
        <Button
          variant="ghost"
          icon={<IconArrowForwardUp stroke={1.6} />}
          disabled={state.future.length === 0}
          aria-label="Redo"
          title="Redo"
          onClick={() => dispatch({ type: "redo" })}
        />
      </ButtonGroup>
      <Button
        variant="panel"
        icon={<IconArrowsExchange stroke={1.6} />}
        disabled={state.waypoints.length < 2}
        aria-label="Reverse"
        title="Reverse"
        onClick={() => dispatch({ type: "reverse" })}
      />
    </div>
  );
}

const STOPS = [
  { key: "profile", label: "Profile", icon: <IconMountain size={14} stroke={1.8} /> },
  { key: "ground", label: "Ground", icon: <IconRoad size={14} stroke={1.8} /> },
] as const;

/** The surface's own rows beside the steepness bands, over the ribbon that places them. */
function GroundStop({
  surface,
  line,
  walkedMetres,
}: {
  surface: SurfaceSummary;
  line: Position[];
  walkedMetres: number;
}) {
  const rows = useMemo(
    () => [
      ...surfaceEntries(surface),
      ...steepnessEntries(gradientSharesBySign(line), surface.totalMetres).filter(
        (entry) => entry.share > 0.005,
      ),
      ...(walkedMetres > 0 ? [{ label: "Walked", metres: walkedMetres, colour: "--ink-2" }] : []),
    ],
    [surface, line, walkedMetres],
  );

  return (
    <div className="flex flex-col gap-3 py-3">
      <GroundRibbon
        segments={groundSegments(surface)}
        surface={surface}
        highlight={null}
        onHighlightChange={() => {}}
      />
      <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm tabular-nums">
        {rows.map((entry) => (
          <span key={entry.label} className="flex items-center gap-2">
            <span
              aria-hidden="true"
              className="size-2.5 shrink-0 rounded-[3px]"
              style={{ background: `var(${entry.colour})` }}
            />
            <span className="flex-1 truncate text-[var(--ink-2)]">{entry.label}</span>
            <span>{formatDistance(entry.metres)}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

/**
 * The strip beneath the map: the plan's figures, and the ground it covers read
 * either as its profile or as its surface. The stop switcher appears only
 * where there is a second stop to switch to — a deployment with no surface map
 * classifies nothing, and a lone tab is not a choice.
 */
function PlannerDock({
  open,
  onOpenChange,
  preview,
  profile,
  line,
  activeMetres,
  onActiveChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preview: PlanRoutePreview | null;
  profile: ReturnType<typeof buildProfile>;
  line: Position[];
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
}) {
  const [stop, setStop] = useState<"profile" | "ground">("profile");
  const surface = useMemo(
    () =>
      preview?.surface && preview.surface.matchedMetres > 0
        ? summariseSurface(line, preview.surface.ranges)
        : null,
    [line, preview],
  );
  // A plan whose ground is unclassified falls back to its profile, including
  // one that was classified until its waypoints moved off the mapped region.
  const showing = surface === null ? "profile" : stop;

  return (
    <section
      aria-label="Planned route"
      className="border-[var(--rule)] border-t bg-[var(--panel)] px-4 py-2"
    >
      <Tabs.Root value={showing} onValueChange={(next) => setStop(next as "profile" | "ground")}>
        <div className="flex items-center justify-between gap-3">
          {preview ? (
            <PlanFigures preview={preview} />
          ) : (
            <span className="font-medium text-sm">Elevation</span>
          )}
          <div className="flex items-center gap-2">
            {surface === null ? null : (
              <SegmentedTrack active={showing}>
                <Tabs.List className="contents">
                  {STOPS.map((item) => (
                    <Tabs.Tab
                      key={item.key}
                      value={item.key}
                      data-segment={item.key}
                      className={segmentClass("sm")}
                    >
                      {item.icon}
                      <SegmentLabel>{item.label}</SegmentLabel>
                    </Tabs.Tab>
                  ))}
                </Tabs.List>
              </SegmentedTrack>
            )}
            <Button
              variant="ghost"
              icon={
                open ? <IconLayoutBottombarCollapse stroke={1.6} /> : <IconMountain stroke={1.6} />
              }
              aria-expanded={open}
              aria-label={open ? "Hide the route detail" : "Show the route detail"}
              onClick={() => onOpenChange(!open)}
            >
              {open ? "Hide" : "Show"}
            </Button>
          </div>
        </div>
        {open ? (
          <>
            <Tabs.Panel value="profile">
              <ElevationProfile
                title="Planned route elevation"
                profile={profile}
                activeMetres={activeMetres}
                onActiveChange={onActiveChange}
              />
            </Tabs.Panel>
            {surface === null ? null : (
              <Tabs.Panel value="ground">
                <GroundStop surface={surface} line={line} walkedMetres={pushingMetres(preview)} />
              </Tabs.Panel>
            )}
          </>
        ) : null}
      </Tabs.Root>
    </section>
  );
}

/** The admin-only planning route. Its guard lives in `App`, beside the other client routes. */
export function PlanPage() {
  const { planId: value } = useParams();
  const planId = value && /^\d+$/.test(value) ? Number(value) : null;
  const location = useLocation();
  const copySeed = planId === null && isPlannerSeed(location.state) ? location.state : null;
  const config = useQuery(webUIConfigQuery());
  const plan = useGetPlan(planId ?? 0, { query: { enabled: planId !== null } });
  const create = useCreatePlan();
  const replace = useReplacePlan();
  const remove = useDeletePlan();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [state, dispatch] = useReducer(plannerReducer, initialPlannerState);
  const [preview, setPreview] = useState<PlanRoutePreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [dockOpen, setDockOpen] = useState(true);
  const [avoidArmed, setAvoidArmed] = useState(false);
  const [focusId, setFocusId] = useState<number | null>(null);
  const [hiddenRun, setHiddenRun] = useState<HiddenRun | null>(null);
  // A resize handle mid-drag: where it is, so the marker and the circle follow it.
  const [resizing, setResizing] = useState<{
    id: number;
    longitude: number;
    latitude: number;
  } | null>(null);
  useEscapeKey(avoidArmed, () => setAvoidArmed(false));
  // Below the breakpoint the strip lives in the Drawer, so it never resizes the map.
  const narrow = useNarrowViewport();
  const [savedPlan, setSavedPlan] = useState<Plan | null>(null);
  const { mutate: previewRoute } = usePreviewPlanRoute();
  const loaded = useRef<string | null>(null);
  const initialViewport = useRef<PlannerFraming | null>(null);
  const hydrating = useRef(planId !== null);
  const request = useRef(0);
  const queryPlan = plan.data?.data;
  const loadedPlan =
    savedPlan?.id === planId ? savedPlan : queryPlan?.id === planId ? queryPlan : undefined;
  const [themeChoice] = useThemeChoice();
  const [basemapChoice, chooseBasemap] = useBasemapChoice();
  const [basemapPickerOpen, setBasemapPickerOpen] = useState(false);
  const prefersDark = usePrefersDarkScheme();
  const basemap = config.data
    ? basemapFor(config.data, resolvesDark(themeChoice, prefersDark), basemapChoice)
    : null;
  // Only a blank draft frames the rider's own position; a plan or seed keeps
  // its own framing, and the request is not made until a blank one is shown.
  const position = useStartupLocation(planId === null && copySeed === null);
  const locationBox = useMemo(() => (position ? boxAround(position) : null), [position]);

  useEffect(() => {
    loaded.current = null;
    initialViewport.current = null;
    hydrating.current = planId !== null;
    request.current += 1;
    setSavedPlan(null);
    dispatch({ type: "reset" });
    if (planId === null) {
      setPreview(null);
      if (copySeed) {
        initialViewport.current = framing(null, copySeed);
        dispatch({ type: "load", plan: copySeed });
      }
    }
    setPreviewError(null);
    setSaveError(null);
    setActiveMetres(null);
    setFocusId(null);
    setHiddenRun(null);
  }, [copySeed, planId]);

  useEffect(() => {
    if (!loadedPlan) {
      return;
    }
    const key = `${loadedPlan.id}/${loadedPlan.version}`;
    if (loaded.current === key) {
      return;
    }
    loaded.current = key;
    if (initialViewport.current?.planId !== loadedPlan.id) {
      initialViewport.current = framing(loadedPlan.id, loadedPlan);
    }
    dispatch({ type: "load", plan: loadedPlan });
    setPreview(previewFrom(loadedPlan));
  }, [loadedPlan]);

  useEffect(() => {
    if (planId !== null && !loadedPlan) {
      request.current += 1;
      return;
    }
    if (state.waypoints.length < 2) {
      request.current += 1;
      if (!hydrating.current) {
        setPreview(null);
      }
      setPreviewError(null);
      return;
    }
    hydrating.current = false;
    const current = ++request.current;
    const timeout = window.setTimeout(() => {
      previewRoute(
        {
          data: {
            profile: state.profile,
            waypoints: planWaypoints(state.waypoints),
            ...(state.avoid.length === 0 ? {} : { avoid: planAvoid(state.avoid) }),
          },
        },
        {
          onSuccess: (response) => {
            if (request.current === current) {
              setPreview(response.data);
              setPreviewError(null);
            }
          },
          onError: (error) => {
            if (request.current === current) {
              setPreviewError(errorMessage(error));
            }
          },
        },
      );
    }, 300);

    return () => window.clearTimeout(timeout);
  }, [loadedPlan, planId, previewRoute, state.profile, state.waypoints, state.avoid]);

  const line = useMemo(() => positions(preview), [preview]);
  const framed = initialViewport.current?.planId === planId ? initialViewport.current.bounds : null;
  // A position that arrives once the rider has started placing waypoints is too
  // late to frame: the camera is theirs by then.
  const blank = planId === null && copySeed === null && state.waypoints.length === 0;
  const viewportBounds = framed ?? (blank ? locationBox : null);
  const profile = useMemo(() => buildProfile(line), [line]);
  const changed =
    !loadedPlan || JSON.stringify(planBody(state)) !== JSON.stringify(planBody(loadedPlan));
  const save = async (published: boolean) => {
    const data = { ...planBody(state), published };
    setSaveError(null);
    try {
      if (planId === null) {
        const response = await create.mutateAsync({ data });
        queryClient.invalidateQueries({ queryKey: getListPlansQueryKey() });
        // A create always stores a draft; publishing it is the replace that follows.
        if (published) {
          await replace
            .mutateAsync({
              planId: response.data.id,
              data,
              headers: { "If-Match": String(response.data.version) },
            })
            .then((replaced) =>
              queryClient.setQueryData(getGetPlanQueryKey(response.data.id), replaced),
            )
            .catch(() => {});
        }
        navigate(`/plan/${response.data.id}`, { replace: true });
        return;
      }
      if (!loadedPlan) {
        return;
      }
      const response = await replace.mutateAsync({
        planId,
        data,
        headers: { "If-Match": String(loadedPlan.version) },
      });
      loaded.current = `${response.data.id}/${response.data.version}`;
      setSavedPlan(response.data);
      dispatch({ type: "load", plan: response.data });
      setPreview(previewFrom(response.data));
      queryClient.setQueryData(getGetPlanQueryKey(planId), response);
      queryClient.invalidateQueries({ queryKey: getListPlansQueryKey() });
      queryClient.invalidateQueries({ queryKey: getGetPlanDeliveryQueryKey(planId) });
    } catch (error) {
      setSaveError(errorMessage(error));
    }
  };
  // A waypoint lands where it was put at once, then settles onto the road beside
  // it when the service finds one; a failed lookup simply leaves it standing.
  const settleOnRoad = (id: number, waypoint: { longitude: number; latitude: number }) => {
    snapPlace(waypoint)
      .then((response) => {
        const place = response.data as SnappedPlace;
        if (place.snapped) {
          dispatch({
            type: "snap",
            id,
            from: waypoint,
            waypoint: { longitude: place.longitude, latitude: place.latitude },
          });
        }
      })
      .catch(() => {});
  };
  const saving = create.isPending || replace.isPending;
  const deletePlan = async () => {
    if (planId === null || !loadedPlan) {
      return;
    }
    setSaveError(null);
    try {
      await remove.mutateAsync({ planId, headers: { "If-Match": String(loadedPlan.version) } });
      queryClient.invalidateQueries({ queryKey: getListPlansQueryKey() });
      navigate("/plan", { replace: true });
    } catch (error) {
      setSaveError(errorMessage(error));
    }
  };

  if (planId !== null && (plan.isPending || plan.isError || !loadedPlan)) {
    return (
      <PageShell>
        {plan.isError ? (
          <Alert variant="destructive">
            <AlertTitle>Could not load plan</AlertTitle>
            <AlertDescription>{errorMessage(plan.error)}</AlertDescription>
          </Alert>
        ) : (
          <p className="text-sm text-[var(--ink-2)]">Loading plan…</p>
        )}
      </PageShell>
    );
  }

  return (
    <Layout
      drawerLabel="Plan a route"
      drawerTitle="Route planner"
      workspaceLabel="Route planner controls"
      workspace="sidebar"
      map={
        <div className="relative size-full">
          {basemap ? (
            <CartographyProvider dark={basemap.dark}>
              <MapWidget
                styleUrl={basemap.styleUrl}
                ariaLabel="Plan route map"
                furniture={
                  <>
                    <ScaleControl position="bottom-left" unit="metric" />
                    <PlannerHistoryControls state={state} dispatch={dispatch} />
                    <MapControls>
                      <Button
                        variant="panel"
                        icon={<IconBan stroke={1.8} />}
                        active={avoidArmed}
                        aria-pressed={avoidArmed}
                        aria-label={avoidArmed ? "Cancel avoiding an area" : "Avoid an area"}
                        title={
                          avoidArmed
                            ? "Click the map to place it, or Esc to cancel"
                            : "Avoid an area"
                        }
                        onClick={() => setAvoidArmed((armed) => !armed)}
                      />
                      <BasemapPicker
                        basemaps={config.data?.basemaps ?? []}
                        selectedName={basemap.name}
                        onSelect={chooseBasemap}
                        expanded={basemapPickerOpen}
                        onExpandedChange={setBasemapPickerOpen}
                      />
                    </MapControls>
                  </>
                }
                cursor={state.waypoints.length === 50 && !avoidArmed ? "" : "crosshair"}
                onClick={(event) => {
                  const point = unwrapped({
                    longitude: event.lngLat.lng,
                    latitude: event.lngLat.lat,
                  });
                  if (avoidArmed) {
                    dispatch({
                      type: "addAvoid",
                      longitude: point.longitude,
                      latitude: point.latitude,
                    });
                    setAvoidArmed(false);
                    return;
                  }
                  if (event.originalEvent.altKey || state.waypoints.length < 2) {
                    dispatch({ type: "append", waypoint: point });
                  } else {
                    dispatch({
                      type: "insert",
                      index: insertionIndex(state.waypoints, point),
                      waypoint: point,
                    });
                  }
                  setFocusId(state.nextWaypointID);
                  settleOnRoad(state.nextWaypointID, point);
                }}
              >
                <MapViewport
                  bounds={viewportBounds}
                  maxZoom={framed ? ROUTE_MAX_ZOOM : LOCATION_ZOOM}
                  // The strip changes height when it folds and when a profile
                  // first fills it; the map re-frames after either.
                  fitRevision={narrow || !dockOpen ? 0 : profile ? 2 : 1}
                />
                {line.length > 1 ? (
                  <RouteOverlay
                    coordinates={line}
                    surface={
                      preview?.surface && preview.surface.matchedMetres > 0
                        ? preview.surface.ranges
                        : undefined
                    }
                    profile={profile}
                    activeProfile={profile}
                    activeMetres={activeMetres}
                    onActiveChange={setActiveMetres}
                    showTerminals={false}
                  />
                ) : null}
                {line.length > 1 && preview?.pushing ? (
                  <PushingLine line={line} pushing={preview.pushing} />
                ) : null}
                {line.length > 1 ? (
                  <HiddenRunLayer line={line} preview={preview} run={hiddenRun} />
                ) : null}
                <AvoidAreasLayer
                  areas={state.avoid.map((area) =>
                    area.id === resizing?.id
                      ? { ...area, radiusMetres: avoidRadiusTo(area, resizing) }
                      : area,
                  )}
                />
                {state.waypoints.map((waypoint, index) => (
                  <Marker
                    key={waypoint.id}
                    longitude={waypoint.longitude}
                    latitude={waypoint.latitude}
                    draggable
                    onDragEnd={(event) => {
                      const moved = unwrapped({
                        longitude: event.lngLat.lng,
                        latitude: event.lngLat.lat,
                      });
                      dispatch({ type: "move", index, waypoint: moved });
                      setFocusId(waypoint.id);
                      if (!waypoint.straight) {
                        settleOnRoad(waypoint.id, moved);
                      }
                    }}
                  >
                    <span
                      role="img"
                      aria-label={waypointLabel(index, state.waypoints.length)}
                      onMouseEnter={() => setFocusId(waypoint.id)}
                      className="grid size-6 place-items-center rounded-full bg-[var(--accent)] text-xs font-semibold text-white shadow"
                    >
                      <WaypointMarker index={index} count={state.waypoints.length} />
                    </span>
                  </Marker>
                ))}
                {state.avoid.map((area) => (
                  <Marker
                    key={area.id}
                    longitude={area.longitude}
                    latitude={area.latitude}
                    draggable
                    onDragEnd={(event) => {
                      const moved = unwrapped({
                        longitude: event.lngLat.lng,
                        latitude: event.lngLat.lat,
                      });
                      dispatch({
                        type: "moveAvoid",
                        id: area.id,
                        longitude: moved.longitude,
                        latitude: moved.latitude,
                      });
                    }}
                  >
                    <span
                      role="img"
                      aria-label="Avoided area centre"
                      className="grid size-4 place-items-center rounded-full bg-[var(--alert)] shadow"
                    />
                  </Marker>
                ))}
                {state.avoid.map((area) => {
                  const edge = resizing?.id === area.id ? resizing : avoidEdge(area);
                  return (
                    <Marker
                      key={`edge-${area.id}`}
                      longitude={edge.longitude}
                      latitude={edge.latitude}
                      draggable
                      onDrag={(event) =>
                        setResizing({
                          id: area.id,
                          longitude: event.lngLat.lng,
                          latitude: event.lngLat.lat,
                        })
                      }
                      onDragEnd={(event) => {
                        setResizing(null);
                        dispatch({
                          type: "setAvoidRadius",
                          id: area.id,
                          radiusMetres: avoidRadiusTo(area, {
                            longitude: event.lngLat.lng,
                            latitude: event.lngLat.lat,
                          }),
                        });
                      }}
                    >
                      <span
                        role="img"
                        aria-label="Avoided area edge, drag to resize"
                        className="block size-3 cursor-ew-resize rounded-full border-2 border-[var(--alert)] bg-white shadow"
                      />
                    </Marker>
                  );
                })}
              </MapWidget>
            </CartographyProvider>
          ) : null}
          {previewError ? (
            <Alert
              variant="destructive"
              className="absolute top-1/2 right-3 z-30 max-w-sm -translate-y-1/2"
            >
              <AlertTitle>Preview unavailable</AlertTitle>
              <AlertDescription>{previewError}</AlertDescription>
            </Alert>
          ) : null}
        </div>
      }
      dock={
        <PlannerDock
          open={dockOpen}
          onOpenChange={setDockOpen}
          preview={preview}
          profile={profile}
          line={line}
          activeMetres={activeMetres}
          onActiveChange={setActiveMetres}
        />
      }
    >
      <PlannerSidebar
        state={state}
        preview={preview}
        planId={planId}
        published={loadedPlan?.published ?? false}
        changed={changed}
        saving={saving}
        deleting={remove.isPending}
        saveError={saveError}
        focusId={focusId}
        {...(loadedPlan?.turnCount === undefined ? {} : { turnCount: loadedPlan.turnCount })}
        onSave={(published) => void save(published)}
        onDelete={() => void deletePlan()}
        onHighlight={setHiddenRun}
        dispatch={dispatch}
      />
    </Layout>
  );
}
