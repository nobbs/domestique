import {
  IconArrowBackUp,
  IconArrowForwardUp,
  IconArrowsExchange,
  IconChevronsRight,
  IconDeviceFloppy,
  IconFlagCheck,
  IconGripVertical,
  IconLayoutBottombarCollapse,
  IconMountain,
  IconPlayerPlay,
  IconTrash,
} from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type Dispatch, useEffect, useMemo, useReducer, useRef, useState } from "react";
import { Marker, ScaleControl } from "react-map-gl/maplibre";
import { Link, useLocation, useNavigate, useParams } from "react-router";
import {
  getGetPlanQueryKey,
  getListPlansQueryKey,
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
  Position,
} from "../../api/types";
import { PLAN_PROFILES } from "../../api/types";
import { Button, ButtonLink } from "../../components/Button";
import { Layout, PageShell } from "../../components/Layout";
import { BasemapPicker } from "../../components/map/BasemapPicker";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapControls } from "../../components/map/MapControls";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { Alert, AlertDescription, AlertTitle } from "../../components/ui/alert";
import { Badge } from "../../components/ui/badge";
import { ButtonGroup } from "../../components/ui/button-group";
import { Input } from "../../components/ui/input";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import { ROUTE_MAX_ZOOM } from "../../lib/cartography";
import { formatAscent, formatDistance } from "../../lib/format";
import { useOverlayInsets } from "../../lib/overlayInsets";
import { buildProfile, rangeBounds } from "../../lib/profile";
import { boxAround, LOCATION_ZOOM, useStartupLocation } from "../../lib/startupLocation";
import { resolvesDark, useThemeChoice } from "../../lib/theme";
import { ElevationProfile } from "../routes/ElevationProfile";
import { RouteOverlay } from "../routes/RouteOverlay";
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
    ...(plan.surface === undefined ? {} : { surface: plan.surface }),
  };
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

interface CoordinateInputProps {
  label: string;
  value: number;
  min: number;
  max: number;
  onCommit: (value: number) => void;
}

function CoordinateInput({ label, value, min, max, onCommit }: CoordinateInputProps) {
  const [raw, setRaw] = useState(String(value));

  useEffect(() => setRaw(String(value)), [value]);

  return (
    <Input
      className="border-transparent bg-transparent hover:bg-[var(--panel)] focus-visible:border-ring"
      type="text"
      inputMode="decimal"
      aria-label={label}
      value={raw}
      onChange={(event) => setRaw(event.target.value)}
      onBlur={() => {
        const complete = raw.trim();
        const coordinate = Number(complete);
        if (
          complete !== "" &&
          Number.isFinite(coordinate) &&
          coordinate >= min &&
          coordinate <= max
        ) {
          if (coordinate !== value) {
            onCommit(coordinate);
          }
        } else {
          setRaw(String(value));
        }
      }}
    />
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
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  onSave: (published: boolean) => void;
  dispatch: Dispatch<Parameters<typeof plannerReducer>[1]>;
}

/** The controls over the map; the map remains visible while the list changes. */
export function PlannerSidebar({
  state,
  plans,
  preview,
  planId,
  published,
  saving,
  saveError,
  collapsed,
  onCollapsedChange,
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
  const clearDrag = () => {
    dragging.current = null;
    dragTarget.current = null;
    dragOrder.current = null;
    setVisualOrder(null);
  };

  return (
    <div data-compact-workspace="" className="w-fit max-w-full">
      <section
        aria-label="Route planner controls"
        className={`max-h-[calc(100dvh-9rem)] max-w-full overflow-y-auto rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] ring-1 ring-black/5 ${collapsed ? "w-fit" : "w-[24rem]"}`}
      >
        <div className="flex items-center gap-1 p-1.5">
          <button
            type="button"
            aria-expanded={!collapsed}
            aria-label={collapsed ? "Show planner controls" : "Hide planner controls"}
            onClick={() => onCollapsedChange(!collapsed)}
            className="flex min-w-0 items-center gap-2 rounded-lg px-2 py-1 text-left hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
          >
            <IconChevronsRight
              size={16}
              stroke={2}
              aria-hidden="true"
              className={collapsed ? "transition-transform" : "rotate-90 transition-transform"}
            />
            <span className="font-semibold">{planId === null ? "Plan a route" : "Edit plan"}</span>
            {collapsed && preview ? (
              <span className="shrink-0 text-sm text-[var(--ink-2)] tabular-nums">
                {formatDistance(preview.distanceMetres)} · {formatAscent(preview.ascentMetres)}
              </span>
            ) : null}
          </button>
          {collapsed ? null : (
            <ButtonLink variant="ghost" className="ml-auto" to="/plan">
              New
            </ButtonLink>
          )}
        </div>
        {collapsed ? null : (
          <div className="grid gap-4 px-4 pt-2 pb-4">
            <label className="grid gap-1 text-sm font-medium">
              Name
              <Input
                value={state.name}
                maxLength={120}
                onChange={(event) => dispatch({ type: "setName", name: event.target.value })}
              />
            </label>
            <label className="grid gap-1 text-sm font-medium">
              Route type
              <select
                value={state.profile}
                onChange={(event) =>
                  dispatch({ type: "setProfile", profile: event.target.value as PlanProfile })
                }
                className="h-8 w-full rounded-lg border border-transparent bg-transparent px-2.5 py-1 text-sm outline-none hover:bg-[var(--panel)] focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
              >
                {PLAN_PROFILES.map((profile) => (
                  <option key={profile} value={profile}>
                    {profile}
                  </option>
                ))}
              </select>
            </label>
            <ol className="grid gap-1" aria-label="Waypoints">
              {visualWaypoints.map((waypoint, index) => {
                const stateIndex = state.waypoints.indexOf(waypoint);

                return (
                  <li
                    key={waypoint.id}
                    data-waypoint-id={waypoint.id}
                    className="flex items-center gap-1 text-sm"
                  >
                    <div
                      role="group"
                      draggable
                      aria-label={`Drag ${waypointLabel(index, state.waypoints.length)} to reorder`}
                      title={`Drag ${waypointLabel(index, state.waypoints.length)} to reorder`}
                      className={`flex min-w-0 flex-1 items-center gap-1 rounded-lg border border-transparent bg-[var(--base)] p-1 hover:border-[var(--rule)] ${dragging.current === waypoint.id ? "opacity-60" : ""}`}
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
                        className="grid size-6 shrink-0 place-items-center rounded-full bg-[var(--accent)] text-xs font-semibold text-white"
                      >
                        <WaypointMarker index={index} count={state.waypoints.length} />
                      </span>
                      <div className="grid min-w-0 flex-1 grid-cols-2 gap-1">
                        <CoordinateInput
                          label={`Waypoint ${index + 1} latitude`}
                          value={waypoint.latitude}
                          min={-90}
                          max={90}
                          onCommit={(latitude) =>
                            dispatch({
                              type: "move",
                              index: stateIndex,
                              waypoint: { ...waypoint, latitude },
                            })
                          }
                        />
                        <CoordinateInput
                          label={`Waypoint ${index + 1} longitude`}
                          value={waypoint.longitude}
                          min={-180}
                          max={180}
                          onCommit={(longitude) =>
                            dispatch({
                              type: "move",
                              index: stateIndex,
                              waypoint: { ...waypoint, longitude },
                            })
                          }
                        />
                      </div>
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
                      <span
                        aria-hidden="true"
                        className="grid size-7 shrink-0 cursor-grab place-items-center rounded-md text-[var(--ink-2)]"
                      >
                        <IconGripVertical aria-hidden="true" size={16} stroke={2} />
                      </span>
                    </div>
                    <Button
                      variant="ghost"
                      icon={<IconTrash size={16} />}
                      className="text-destructive hover:bg-destructive/10 hover:text-destructive focus-visible:text-destructive"
                      aria-label={`Delete waypoint ${index + 1}`}
                      onClick={() => dispatch({ type: "delete", index: stateIndex })}
                    />
                  </li>
                );
              })}
            </ol>
            <p className="text-sm text-[var(--ink-2)]">
              Click the map to insert a waypoint, or Alt-click to append. Drag waypoint rows to
              reorder; drag map pins to move them.
            </p>
            {preview ? (
              <output
                aria-label="Planned route summary"
                className="text-sm font-medium tabular-nums"
              >
                {formatDistance(preview.distanceMetres)} · {formatAscent(preview.ascentMetres)}
              </output>
            ) : null}
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
            <div className="grid gap-1 border-[var(--rule)] border-t pt-3">
              <h2 className="text-sm font-medium">Plans</h2>
              {plans.map((plan) => (
                <Link
                  key={plan.id}
                  to={`/plan/${plan.id}`}
                  className="flex items-center justify-between rounded-md px-2 py-1 text-sm hover:bg-[var(--base)]"
                >
                  <span>{plan.name}</span>
                  {plan.published ? null : <Badge variant="secondary">Draft</Badge>}
                </Link>
              ))}
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

function PlannerHistoryControls({
  state,
  dispatch,
  collapsed,
  insetLeft,
}: Pick<PlannerSidebarProps, "state" | "dispatch"> & { collapsed: boolean; insetLeft: number }) {
  return (
    <div
      className="pointer-events-auto absolute z-10 flex items-center gap-2"
      style={{ left: collapsed ? 12 : insetLeft + 12, top: collapsed ? 60 : 12 }}
    >
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

function PlannerDock({
  open,
  onOpenChange,
  preview,
  profile,
  activeMetres,
  onActiveChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preview: PlanRoutePreview | null;
  profile: ReturnType<typeof buildProfile>;
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
}) {
  if (!open) {
    return (
      <Button
        variant="panel"
        icon={<IconMountain stroke={1.6} />}
        aria-expanded="false"
        aria-label="Show elevation"
        onClick={() => onOpenChange(true)}
      >
        Show elevation
      </Button>
    );
  }

  return (
    <section
      aria-label="Planned route elevation"
      className="w-full max-w-5xl rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5"
    >
      <div className="mb-2 flex items-center justify-between gap-3">
        {preview ? (
          <output aria-label="Elevation summary" className="text-sm font-medium tabular-nums">
            {formatDistance(preview.distanceMetres)} · {formatAscent(preview.ascentMetres)}
          </output>
        ) : (
          <span className="text-sm font-medium">Elevation</span>
        )}
        <Button
          variant="ghost"
          icon={<IconLayoutBottombarCollapse stroke={1.6} />}
          aria-expanded="true"
          aria-label="Hide elevation"
          onClick={() => onOpenChange(false)}
        >
          Hide
        </Button>
      </div>
      <ElevationProfile
        title="Planned route elevation"
        profile={profile}
        activeMetres={activeMetres}
        onActiveChange={onActiveChange}
      />
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
  const [panelCollapsed, setPanelCollapsed] = useState(false);
  const [dockOpen, setDockOpen] = useState(true);
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
  const insets = useOverlayInsets();
  // Decided once, at mount: only a blank draft frames the rider's own position,
  // and a plan or seed that opens later keeps its own framing.
  const [locationEnabled] = useState(() => planId === null && copySeed === null);
  const position = useStartupLocation(locationEnabled);
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
  const viewportBounds = framed ?? (planId === null && copySeed === null ? locationBox : null);
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
                    <PlannerHistoryControls
                      state={state}
                      dispatch={dispatch}
                      collapsed={panelCollapsed}
                      insetLeft={insets.left}
                    />
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
                }}
              >
                <MapViewport
                  bounds={viewportBounds}
                  maxZoom={framed ? ROUTE_MAX_ZOOM : LOCATION_ZOOM}
                  insets={insets}
                />
                {line.length > 1 ? (
                  <RouteOverlay
                    coordinates={line}
                    surface={preview?.surface?.ranges}
                    profile={profile}
                    activeProfile={profile}
                    activeMetres={activeMetres}
                    onActiveChange={setActiveMetres}
                    showTerminals={false}
                  />
                ) : null}
                {state.waypoints.map((waypoint, index) => (
                  <Marker
                    key={waypoint.id}
                    longitude={waypoint.longitude}
                    latitude={waypoint.latitude}
                    draggable
                    onDragEnd={(event) =>
                      dispatch({
                        type: "move",
                        index,
                        waypoint: { longitude: event.lngLat.lng, latitude: event.lngLat.lat },
                      })
                    }
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
        collapsed={panelCollapsed}
        onCollapsedChange={setPanelCollapsed}
        onSave={(published) => void save(published)}
        dispatch={dispatch}
      />
    </Layout>
  );
}
