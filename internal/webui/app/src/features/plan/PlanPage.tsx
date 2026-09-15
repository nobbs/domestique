import {
  IconArrowDown,
  IconArrowUp,
  IconDeviceFloppy,
  IconMapPin,
  IconPlayerTrackNext,
  IconPlayerTrackPrev,
  IconRestore,
  IconTrash,
} from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type Dispatch, useEffect, useMemo, useReducer, useRef, useState } from "react";
import { Marker } from "react-map-gl/maplibre";
import { Link, useNavigate, useParams } from "react-router";
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
import type { Plan, PlanProfile, PlanRoutePreview, PlanSummary, Position } from "../../api/types";
import { PLAN_PROFILES } from "../../api/types";
import { Button, ButtonLink } from "../../components/Button";
import { PageShell } from "../../components/Layout";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapWidget } from "../../components/map/MapWidget";
import { Alert, AlertDescription, AlertTitle } from "../../components/ui/alert";
import { Badge } from "../../components/ui/badge";
import { Input } from "../../components/ui/input";
import { RadioGroup, RadioGroupItem } from "../../components/ui/radio-group";
import { formatAscent, formatDistance } from "../../lib/format";
import { buildProfile } from "../../lib/profile";
import { ElevationProfile } from "../routes/ElevationProfile";
import { RouteOverlay } from "../routes/RouteOverlay";
import { initialPlannerState, type PlannerState, plannerReducer } from "./planner";

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
  };
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Could not route these waypoints.";
}

function planWaypoints(waypoints: PlannerState["waypoints"]) {
  return waypoints.map(({ longitude, latitude }) => ({ longitude, latitude }));
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

/** The controls over the map; the map remains visible while the list changes. */
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
  return (
    <aside className="grid gap-4 rounded-xl border border-[var(--rule)] bg-[var(--panel)] p-4 shadow-[var(--shadow)]">
      <div className="flex items-center justify-between gap-2">
        <h1 className="text-lg font-semibold">{planId === null ? "Plan a route" : "Edit plan"}</h1>
        <ButtonLink variant="ghost" to="/plan">
          New
        </ButtonLink>
      </div>
      <label className="grid gap-1 text-sm font-medium">
        Name
        <Input
          value={state.name}
          maxLength={120}
          onChange={(event) => dispatch({ type: "setName", name: event.target.value })}
        />
      </label>
      <fieldset className="grid gap-1">
        <legend className="text-sm font-medium">Route type</legend>
        <RadioGroup
          value={state.profile}
          onValueChange={(profile) =>
            dispatch({ type: "setProfile", profile: profile as PlanProfile })
          }
          className="flex flex-wrap gap-2"
        >
          {PLAN_PROFILES.map((profile) => (
            <label key={profile} className="flex items-center gap-1.5 text-sm">
              <RadioGroupItem value={profile} />
              {profile}
            </label>
          ))}
        </RadioGroup>
      </fieldset>
      <div className="flex flex-wrap gap-1">
        <Button
          variant="outline"
          icon={<IconPlayerTrackPrev stroke={1.6} />}
          disabled={state.past.length === 0}
          onClick={() => dispatch({ type: "undo" })}
        >
          Undo
        </Button>
        <Button
          variant="outline"
          icon={<IconPlayerTrackNext stroke={1.6} />}
          disabled={state.future.length === 0}
          onClick={() => dispatch({ type: "redo" })}
        >
          Redo
        </Button>
        <Button
          variant="outline"
          icon={<IconRestore stroke={1.6} />}
          disabled={state.waypoints.length < 2}
          onClick={() => dispatch({ type: "reverse" })}
        >
          Reverse
        </Button>
      </div>
      <ol className="grid gap-1" aria-label="Waypoints">
        {state.waypoints.map((waypoint, index) => (
          <li key={waypoint.id} className="flex items-center gap-1 text-sm">
            <IconMapPin size={16} aria-hidden="true" />
            <span className="min-w-0 flex-1 truncate">{`${waypoint.latitude.toFixed(5)}, ${waypoint.longitude.toFixed(5)}`}</span>
            <Button
              variant="ghost"
              icon={<IconArrowUp size={16} />}
              aria-label={`Move waypoint ${index + 1} up`}
              disabled={index === 0}
              onClick={() => dispatch({ type: "reorder", index, direction: "up" })}
            />
            <Button
              variant="ghost"
              icon={<IconArrowDown size={16} />}
              aria-label={`Move waypoint ${index + 1} down`}
              disabled={index === state.waypoints.length - 1}
              onClick={() => dispatch({ type: "reorder", index, direction: "down" })}
            />
            <Button
              variant="ghost"
              icon={<IconTrash size={16} />}
              aria-label={`Delete waypoint ${index + 1}`}
              onClick={() => dispatch({ type: "delete", index })}
            />
          </li>
        ))}
      </ol>
      <p className="text-sm text-[var(--ink-2)]">
        Click the map to add up to 50 waypoints. Drag a pin to move it.
      </p>
      {preview ? (
        <output aria-label="Planned route summary" className="text-sm font-medium tabular-nums">
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
    </aside>
  );
}

/** The admin-only planning route. Its guard lives in `App`, beside the other client routes. */
export function PlanPage() {
  const { planId: value } = useParams();
  const planId = value && /^\d+$/.test(value) ? Number(value) : null;
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
  const { mutate: previewRoute } = usePreviewPlanRoute();
  const loaded = useRef<string | null>(null);
  const request = useRef(0);
  const loadedPlan = plan.data?.data;

  useEffect(() => {
    if (planId !== null) {
      return;
    }
    loaded.current = null;
    request.current += 1;
    dispatch({ type: "reset" });
    setPreview(null);
    setPreviewError(null);
    setSaveError(null);
    setActiveMetres(null);
  }, [planId]);

  useEffect(() => {
    if (!loadedPlan) {
      return;
    }
    const key = `${loadedPlan.id}/${loadedPlan.version}`;
    if (loaded.current === key) {
      return;
    }
    loaded.current = key;
    dispatch({ type: "load", plan: loadedPlan });
    setPreview(previewFrom(loadedPlan));
  }, [loadedPlan]);

  useEffect(() => {
    if (state.waypoints.length < 2) {
      request.current += 1;
      setPreview(null);
      setPreviewError(null);
      return;
    }
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
  }, [previewRoute, state.profile, state.waypoints]);

  const line = useMemo(() => positions(preview), [preview]);
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
      const response = await replace.mutateAsync({
        planId,
        data,
        headers: { "If-Match": String(loadedPlan?.version ?? 0) },
      });
      loaded.current = `${response.data.id}/${response.data.version}`;
      dispatch({ type: "load", plan: response.data });
      setPreview(previewFrom(response.data));
      queryClient.invalidateQueries({ queryKey: getGetPlanQueryKey(planId) });
      queryClient.invalidateQueries({ queryKey: getListPlansQueryKey() });
    } catch (error) {
      setSaveError(errorMessage(error));
    }
  };
  const saving = create.isPending || replace.isPending;
  const styleUrl = config.data?.basemaps[0]?.styleUrl;

  return (
    <PageShell>
      <div className="grid min-h-[calc(100dvh-9rem)] gap-4 lg:grid-cols-[24rem_1fr]">
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
        <div className="relative min-h-96 overflow-hidden rounded-xl">
          {styleUrl ? (
            <CartographyProvider dark={config.data?.basemaps[0]?.darkCartography ?? false}>
              <MapWidget
                styleUrl={styleUrl}
                ariaLabel="Plan route map"
                cursor={state.waypoints.length === 50 ? "" : "crosshair"}
                onClick={(event) =>
                  dispatch({
                    type: "append",
                    waypoint: { longitude: event.lngLat.lng, latitude: event.lngLat.lat },
                  })
                }
              >
                {line.length > 1 ? (
                  <RouteOverlay
                    coordinates={line}
                    profile={profile}
                    activeProfile={profile}
                    activeMetres={activeMetres}
                    onActiveChange={setActiveMetres}
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
                    <span className="grid size-6 place-items-center rounded-full bg-[var(--accent)] text-xs font-semibold text-white shadow">
                      {index + 1}
                    </span>
                  </Marker>
                ))}
              </MapWidget>
            </CartographyProvider>
          ) : null}
          {previewError ? (
            <Alert variant="destructive" className="absolute right-3 bottom-3 left-3">
              <AlertTitle>Preview unavailable</AlertTitle>
              <AlertDescription>{previewError}</AlertDescription>
            </Alert>
          ) : null}
        </div>
        <div className="lg:col-start-2">
          <ElevationProfile
            title="Planned route elevation"
            profile={profile}
            activeMetres={activeMetres}
            onActiveChange={setActiveMetres}
          />
        </div>
      </div>
    </PageShell>
  );
}
