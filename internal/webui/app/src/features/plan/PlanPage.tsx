import { Tabs } from "@base-ui/react/tabs";
import {
  IconArrowBackUp,
  IconArrowDownRight,
  IconArrowForwardUp,
  IconArrowsExchange,
  IconArrowUpRight,
  IconBike,
  IconChevronDown,
  IconClock,
  IconDeviceFloppy,
  IconFlagCheck,
  IconGripVertical,
  IconLayoutBottombarCollapse,
  IconMountain,
  IconPlayerPlay,
  IconRoad,
  IconRoute,
  IconTrash,
  IconWalk,
} from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type Dispatch,
  type ReactNode,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";
import { Layer, Marker, ScaleControl, Source } from "react-map-gl/maplibre";
import { Link, useLocation, useNavigate, useParams } from "react-router";
import {
  getGetPlanQueryKey,
  getListPlansQueryKey,
  snapPlace,
  useCreatePlan,
  useGetPlan,
  useListPlans,
  usePreviewPlanRoute,
  useReplacePlan,
} from "../../api/generated";
import { webUIConfigQuery } from "../../api/queries";
import type {
  BoundingBox,
  Plan,
  PlanProfile,
  PlanRoutePreview,
  PlanSummary,
  PlanWindow,
  Position,
  SnappedPlace,
} from "../../api/types";
import { Button, ButtonLink } from "../../components/Button";
import { InfoDot } from "../../components/InsetForm";
import { Layout, PageShell } from "../../components/Layout";
import { BasemapPicker } from "../../components/map/BasemapPicker";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapControls } from "../../components/map/MapControls";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { Panel } from "../../components/PanelHeading";
import { Segmented, SegmentedTrack, SegmentLabel, segmentClass } from "../../components/Segmented";
import { Alert, AlertDescription, AlertTitle } from "../../components/ui/alert";
import { Badge } from "../../components/ui/badge";
import { ButtonGroup } from "../../components/ui/button-group";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import { Input } from "../../components/ui/input";
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
import { ElevationProfile } from "../routes/ElevationProfile";
import { GroundRibbon } from "../routes/GroundRibbon";
import { RouteOverlay } from "../routes/RouteOverlay";
import { usePlaceName } from "./placeName";
import {
  initialPlannerState,
  isPlannerSeed,
  type PlannerSeed,
  type PlannerState,
  plannerReducer,
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

function planWaypoints(waypoints: PlannerState["waypoints"]) {
  return waypoints.map(({ longitude, latitude }) => ({ longitude, latitude }));
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

function waypointLabel(index: number, count: number): string {
  if (index === 0) {
    return "Start waypoint";
  }
  if (index === count - 1) {
    return "Finish waypoint";
  }
  return `Waypoint ${index + 1}`;
}

function WaypointMarker({ index, count }: { index: number; count: number }) {
  if (index === 0) {
    return <IconPlayerPlay aria-hidden="true" size={14} stroke={3} />;
  }
  if (index === count - 1) {
    return <IconFlagCheck aria-hidden="true" size={15} stroke={2.5} />;
  }
  return index + 1;
}

function moveWaypoint(order: number[], waypointID: number, targetID: number): number[] {
  const from = order.indexOf(waypointID);
  const target = order.indexOf(targetID);
  if (from < 0 || target < 0 || from === target) {
    return order;
  }
  const next = [...order];
  next.splice(from, 1);
  next.splice(target, 0, waypointID);
  return next;
}

function isInteractiveDragOrigin(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    target.closest("input, button, a, [contenteditable=true]") !== null
  );
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

const PROFILES = [
  { key: "trekking", label: "Trekking", icon: <IconBike size={14} stroke={1.8} /> },
  { key: "fastbike", label: "Road", icon: <IconRoad size={14} stroke={1.8} /> },
  { key: "gravel", label: "Gravel", icon: <IconRoute size={14} stroke={1.8} /> },
] as const satisfies readonly { key: PlanProfile; label: string; icon: ReactNode }[];

const coordinatesOf = (latitude: number, longitude: number) =>
  `${latitude.toFixed(4)}, ${longitude.toFixed(4)}`;

/**
 * What the waypoint is called, falling back to its own position: the service
 * names nothing without a geocoder, and a geocoder names nothing in open
 * country.
 */
function WaypointName({ latitude, longitude }: { latitude: number; longitude: number }) {
  const name = usePlaceName(latitude, longitude);

  return (
    <span className="truncate">{name === "" ? coordinatesOf(latitude, longitude) : name}</span>
  );
}

/** One row's worth of edge, and the fastest the list is nudged, in pixels a frame. */
const EDGE_ZONE = 44;
const EDGE_SPEED = 12;

/**
 * How fast a dragged row should scroll the list it is over: negative up,
 * positive down, zero away from either edge. WebKit scrolls no element but the
 * page during a drag, so the planner does this itself.
 */
export function edgeSpeed(top: number, bottom: number, pointerY: number): number {
  const intoTop = EDGE_ZONE - (pointerY - top);
  const intoBottom = EDGE_ZONE - (bottom - pointerY);
  if (intoTop > 0) {
    return -Math.min(EDGE_SPEED, (intoTop / EDGE_ZONE) * EDGE_SPEED);
  }
  if (intoBottom > 0) {
    return Math.min(EDGE_SPEED, (intoBottom / EDGE_ZONE) * EDGE_SPEED);
  }

  return 0;
}

/** The row's own short word for a stop, where waypointLabel names it for a reader. */
function stopLabel(index: number, count: number): string {
  if (index === 0) {
    return "Start";
  }
  if (index === count - 1) {
    return "Finish";
  }

  return `Via ${index}`;
}

/** How far and how long the routed line has run by the time it reaches a waypoint. */
function progressLabel(preview: PlanRoutePreview | null, index: number): string {
  const at = index === 0 ? undefined : preview?.waypointProgress?.[index];
  if (!at) {
    return "";
  }
  const minutes = Math.round((at.movingSeconds ?? 0) / 60);
  const elapsed = minutes === 0 ? "" : ` · ${formatDuration(minutes * 60)}`;

  return ` · ${formatDistance(at.distanceMetres)}${elapsed}`;
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

export interface PlannerSidebarProps {
  state: PlannerState;
  plans: PlanSummary[];
  preview: PlanRoutePreview | null;
  planId: number | null;
  published: boolean;
  saving: boolean;
  saveError: string | null;
  onSave: (published: boolean) => void;
  dispatch: Dispatch<Parameters<typeof plannerReducer>[1]>;
}

/** The planner's column beside the map; the map remains visible while the list changes. */
export function PlannerSidebar({
  state,
  plans,
  preview,
  planId,
  published,
  saving,
  saveError,
  onSave,
  dispatch,
}: PlannerSidebarProps) {
  const dragging = useRef<number | null>(null);
  const dragTarget = useRef<number | null>(null);
  const dragOrder = useRef<number[] | null>(null);
  const [visualOrder, setVisualOrder] = useState<number[] | null>(null);
  const visualWaypoints = useMemo(() => {
    if (!visualOrder) {
      return state.waypoints;
    }
    const byID = new Map(state.waypoints.map((waypoint) => [waypoint.id, waypoint]));
    return visualOrder.flatMap((id) => byID.get(id) ?? []);
  }, [state.waypoints, visualOrder]);
  const list = useRef<HTMLOListElement>(null);
  const speed = useRef(0);
  const frame = useRef<number | null>(null);
  const step = () => {
    if (!list.current || speed.current === 0) {
      frame.current = null;

      return;
    }
    list.current.scrollTop += speed.current;
    frame.current = requestAnimationFrame(step);
  };
  const scrollNearEdge = (pointerY: number) => {
    if (!list.current) {
      return;
    }
    const box = list.current.getBoundingClientRect();
    speed.current = edgeSpeed(box.top, box.bottom, pointerY);
    if (speed.current !== 0 && frame.current === null) {
      frame.current = requestAnimationFrame(step);
    }
  };
  const stopScrolling = () => {
    speed.current = 0;
    if (frame.current !== null) {
      cancelAnimationFrame(frame.current);
      frame.current = null;
    }
  };
  useEffect(
    () => () => {
      if (frame.current !== null) {
        cancelAnimationFrame(frame.current);
      }
    },
    [],
  );
  const clearDrag = () => {
    dragging.current = null;
    dragTarget.current = null;
    dragOrder.current = null;
    stopScrolling();
    setVisualOrder(null);
  };

  return (
    // The column fills the rail and only the stops scroll, so the actions and
    // the plan's own figures stay in view however long the route is.
    <section className="flex min-h-0 flex-1 flex-col gap-3 bg-[var(--base)] p-3">
      <Panel
        className="min-h-0 flex-1"
        icon={<IconRoute size={18} stroke={1.8} />}
        title={
          <Input
            aria-label="Plan name"
            className="min-w-0 border-transparent border-b-2 bg-transparent px-0 font-semibold text-base shadow-none focus-visible:border-[var(--accent)] focus-visible:ring-0"
            placeholder={planId === null ? "Plan a route" : "Edit plan"}
            value={state.name}
            maxLength={120}
            onChange={(event) => dispatch({ type: "setName", name: event.target.value })}
          />
        }
        aside={
          <div className="flex items-center gap-1">
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" icon={<IconChevronDown size={16} stroke={2} />} />}
                disabled={plans.length === 0}
              >
                Plans
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-auto min-w-52">
                {plans.map((plan) => (
                  <DropdownMenuItem key={plan.id} render={<Link to={`/plan/${plan.id}`} />}>
                    <span className="flex-1">{plan.name}</span>
                    {plan.published ? null : <Badge variant="secondary">Draft</Badge>}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
            <ButtonLink variant="ghost" to="/plan">
              New
            </ButtonLink>
          </div>
        }
      >
        <Segmented
          label="Route type"
          items={PROFILES}
          value={state.profile}
          onChange={(profile) => dispatch({ type: "setProfile", profile })}
        />
        <h3 className="-mb-2 flex items-center gap-2 font-semibold text-sm">
          Waypoints
          <span className="flex-1 font-normal text-[var(--ink-2)] text-xs">
            {state.waypoints.length === 1 ? "1 stop" : `${state.waypoints.length} stops`}
          </span>
          <InfoDot label="Waypoints">
            Click the map to insert a waypoint, or Alt-click to append. Drag waypoint rows to
            reorder; drag map pins to move them.
          </InfoDot>
        </h3>
        <ol
          ref={list}
          onDragOver={(event) => scrollNearEdge(event.clientY)}
          onDragLeave={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
              stopScrolling();
            }
          }}
          // The card pads by 20px; the rows keep 4 of it, so a name has the width.
          // Rows are one height and snap, so scrolling never stops on half a stop.
          // Snapping is off while a row is being dragged: it fights the
          // browser's own scrolling at the list's edges.
          data-dragging={visualOrder === null ? undefined : true}
          className="-mx-4 flex min-h-0 snap-y snap-mandatory flex-col overflow-y-auto rounded-xl bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] empty:hidden data-dragging:snap-none"
          aria-label="Waypoints"
        >
          {visualWaypoints.map((waypoint, index) => {
            const stateIndex = state.waypoints.indexOf(waypoint);

            return (
              <li
                key={waypoint.id}
                data-waypoint-id={waypoint.id}
                // The start and the finish stay in view while the stops between
                // them scroll under, so a long route keeps its ends.
                className="flex h-11 shrink-0 snap-start scroll-mt-11 items-center gap-1 border-[var(--panel)] border-b-2 bg-[color-mix(in_oklab,var(--ink-2)_7%,var(--panel))] pr-1.5 pl-3 text-sm last:sticky last:bottom-0 last:border-b-0 first:sticky first:top-0"
              >
                <div
                  role="group"
                  draggable
                  // No title: the hint under the list says rows drag, and a
                  // tooltip over every row covers the map beside it.
                  aria-label={`Drag ${waypointLabel(index, state.waypoints.length)} to reorder`}
                  className={`flex min-w-0 flex-1 items-center gap-2.5 ${dragging.current === waypoint.id ? "opacity-60" : ""}`}
                  onDragStart={(event) => {
                    if (isInteractiveDragOrigin(event.target)) {
                      event.preventDefault();
                      return;
                    }
                    const order = state.waypoints.map((entry) => entry.id);
                    dragging.current = waypoint.id;
                    dragTarget.current = null;
                    dragOrder.current = order;
                    setVisualOrder(order);
                    event.dataTransfer?.setData("text/plain", String(waypoint.id));
                    if (event.dataTransfer) {
                      event.dataTransfer.effectAllowed = "move";
                      event.dataTransfer.setDragImage(
                        event.currentTarget,
                        Math.round(event.currentTarget.clientWidth / 2),
                        Math.round(event.currentTarget.clientHeight / 2),
                      );
                    }
                  }}
                  onDragOver={(event) => {
                    event.preventDefault();
                    const waypointID = dragging.current;
                    if (waypointID === null || dragTarget.current === waypoint.id) {
                      return;
                    }
                    dragTarget.current = waypoint.id;
                    const next = moveWaypoint(
                      dragOrder.current ?? state.waypoints.map((entry) => entry.id),
                      waypointID,
                      waypoint.id,
                    );
                    if (next !== dragOrder.current) {
                      dragOrder.current = next;
                      setVisualOrder(next);
                    }
                  }}
                  onDrop={(event) => {
                    event.preventDefault();
                    const order = dragOrder.current;
                    clearDrag();
                    if (order) {
                      dispatch({ type: "reorder", order });
                    }
                  }}
                  onDragEnd={clearDrag}
                >
                  <span
                    aria-hidden="true"
                    className={`grid size-6 shrink-0 place-items-center rounded-[8px] font-semibold text-white text-xs ${
                      index === 0 || index === state.waypoints.length - 1
                        ? "bg-[var(--accent)]"
                        : "bg-[var(--ink-2)]"
                    }`}
                  >
                    <WaypointMarker index={index} count={state.waypoints.length} />
                  </span>
                  <span className="flex min-w-0 flex-1 flex-col">
                    <WaypointName latitude={waypoint.latitude} longitude={waypoint.longitude} />
                    <span className="truncate text-[var(--ink-2)] text-xs tabular-nums">
                      {stopLabel(index, state.waypoints.length)}
                      {progressLabel(preview, stateIndex)}
                    </span>
                  </span>
                  <Button
                    variant="ghost"
                    className="sr-only focus:not-sr-only"
                    aria-label={`Move ${waypointLabel(index, state.waypoints.length)} up`}
                    disabled={index === 0}
                    onClick={() =>
                      dispatch({ type: "reorder", index: stateIndex, direction: "up" })
                    }
                  >
                    Move up
                  </Button>
                  <Button
                    variant="ghost"
                    className="sr-only focus:not-sr-only"
                    aria-label={`Move ${waypointLabel(index, state.waypoints.length)} down`}
                    disabled={index === state.waypoints.length - 1}
                    onClick={() =>
                      dispatch({ type: "reorder", index: stateIndex, direction: "down" })
                    }
                  >
                    Move down
                  </Button>
                  <IconGripVertical
                    aria-hidden="true"
                    size={15}
                    className="shrink-0 cursor-grab text-[var(--ink-2)]"
                  />
                </div>
                <Button
                  variant="ghost"
                  icon={<IconTrash size={15} />}
                  aria-label={`Delete waypoint ${index + 1}`}
                  onClick={() => dispatch({ type: "delete", index: stateIndex })}
                />
              </li>
            );
          })}
        </ol>
      </Panel>
      <div className="grid shrink-0 gap-2">
        <div className="flex flex-wrap gap-2">
          <Button
            icon={<IconDeviceFloppy stroke={1.6} />}
            disabled={saving || state.name.trim() === "" || state.waypoints.length < 2}
            onClick={() => onSave(planId === null ? false : published)}
          >
            {planId === null ? "Save draft" : "Save changes"}
          </Button>
          {planId !== null && !published ? (
            <Button
              variant="outline"
              disabled={saving || state.name.trim() === "" || state.waypoints.length < 2}
              onClick={() => onSave(true)}
            >
              Publish — syncs on next run
            </Button>
          ) : null}
        </div>
        {planId !== null && published ? (
          <Button
            variant="warning"
            disabled={saving || state.name.trim() === "" || state.waypoints.length < 2}
            onClick={() => onSave(false)}
          >
            Unpublish — removes on next sync
          </Button>
        ) : null}
        {saveError ? (
          <Alert variant="destructive">
            <AlertTitle>Could not save plan</AlertTitle>
            <AlertDescription>{saveError}</AlertDescription>
          </Alert>
        ) : null}
      </div>
    </section>
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
        className="divide-x divide-[var(--rule)] rounded-lg bg-[var(--panel)] shadow-[var(--shadow)] ring-1 ring-[var(--rule)] ring-inset [&>*:not(:first-child)]:rounded-l-none [&>*:not(:last-child)]:rounded-r-none"
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
  const plans = useListPlans();
  const plan = useGetPlan(planId ?? 0, { query: { enabled: planId !== null } });
  const create = useCreatePlan();
  const replace = useReplacePlan();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [state, dispatch] = useReducer(plannerReducer, initialPlannerState);
  const [preview, setPreview] = useState<PlanRoutePreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [dockOpen, setDockOpen] = useState(true);
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
        { data: { profile: state.profile, waypoints: planWaypoints(state.waypoints) } },
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
  }, [loadedPlan, planId, previewRoute, state.profile, state.waypoints]);

  const line = useMemo(() => positions(preview), [preview]);
  const framed = initialViewport.current?.planId === planId ? initialViewport.current.bounds : null;
  // A position that arrives once the rider has started placing waypoints is too
  // late to frame: the camera is theirs by then.
  const blank = planId === null && copySeed === null && state.waypoints.length === 0;
  const viewportBounds = framed ?? (blank ? locationBox : null);
  const profile = useMemo(() => buildProfile(line), [line]);
  const save = async (published: boolean) => {
    const data = {
      name: state.name.trim(),
      profile: state.profile,
      waypoints: planWaypoints(state.waypoints),
      published,
    };
    setSaveError(null);
    try {
      if (planId === null) {
        const response = await create.mutateAsync({ data });
        queryClient.invalidateQueries({ queryKey: getListPlansQueryKey() });
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
            waypoint: { longitude: place.longitude, latitude: place.latitude },
          });
        }
      })
      .catch(() => {});
  };
  const saving = create.isPending || replace.isPending;

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
                cursor={state.waypoints.length === 50 ? "" : "crosshair"}
                onClick={(event) => {
                  const waypoint = { longitude: event.lngLat.lng, latitude: event.lngLat.lat };
                  if (event.originalEvent.altKey || state.waypoints.length < 2) {
                    dispatch({ type: "append", waypoint });
                  } else {
                    dispatch({
                      type: "insert",
                      index: insertionIndex(state.waypoints, waypoint),
                      waypoint,
                    });
                  }
                  settleOnRoad(state.nextWaypointID, waypoint);
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
                {state.waypoints.map((waypoint, index) => (
                  <Marker
                    key={waypoint.id}
                    longitude={waypoint.longitude}
                    latitude={waypoint.latitude}
                    draggable
                    onDragEnd={(event) => {
                      const moved = { longitude: event.lngLat.lng, latitude: event.lngLat.lat };
                      dispatch({ type: "move", index, waypoint: moved });
                      settleOnRoad(waypoint.id, moved);
                    }}
                  >
                    <span
                      role="img"
                      aria-label={waypointLabel(index, state.waypoints.length)}
                      className="grid size-6 place-items-center rounded-full bg-[var(--accent)] text-xs font-semibold text-white shadow"
                    >
                      <WaypointMarker index={index} count={state.waypoints.length} />
                    </span>
                  </Marker>
                ))}
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
        plans={plans.data?.data.plans ?? []}
        preview={preview}
        planId={planId}
        published={loadedPlan?.published ?? false}
        saving={saving}
        saveError={saveError}
        onSave={(published) => void save(published)}
        dispatch={dispatch}
      />
    </Layout>
  );
}
