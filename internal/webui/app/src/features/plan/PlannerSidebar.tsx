/** The planner's card beside the map: the plan's name, figures and settings, its waypoints, and saving. */

import {
  IconChevronDown,
  IconClock,
  IconFlagCheck,
  IconGripVertical,
  IconMountain,
  IconPencil,
  IconPlayerPlay,
  IconPlus,
  IconRoute,
  IconRoute2,
  IconTrash,
} from "@tabler/icons-react";
import { type ComponentProps, type Dispatch, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import type { PlanProfile, PlanRoutePreview } from "../../api/types";
import { Button } from "../../components/Button";
import { Segmented } from "../../components/Segmented";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "../../components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import { formatAscent, formatCount, formatDistance, formatDuration } from "../../lib/format";
import { cn } from "../../lib/utils";
import { PlanDeliveryTrigger } from "./PlanDelivery";
import { usePlaceName } from "./placeName";
import type { PlannerAvoid, PlannerState, plannerReducer } from "./planner";

const PROFILES = [
  { key: "trekking", label: "Trekking" },
  { key: "fastbike", label: "Road" },
  { key: "gravel", label: "Gravel" },
] as const satisfies readonly { key: PlanProfile; label: string }[];

export function waypointLabel(index: number, count: number): string {
  if (index === 0) {
    return "Start waypoint";
  }
  if (index === count - 1) {
    return "Finish waypoint";
  }
  return `Waypoint ${index + 1}`;
}

export function WaypointMarker({ index, count }: { index: number; count: number }) {
  if (index === 0) {
    return <IconPlayerPlay aria-hidden="true" size={12} stroke={3} />;
  }
  if (index === count - 1) {
    return <IconFlagCheck aria-hidden="true" size={13} stroke={2.5} />;
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

const coordinatesOf = (latitude: number, longitude: number) =>
  `${latitude.toFixed(4)}, ${longitude.toFixed(4)}`;

/**
 * What a place is called, falling back to its own position: the service
 * names nothing without a geocoder, and a geocoder names nothing in open
 * country.
 */
function PlaceName({
  latitude,
  longitude,
  className,
}: {
  latitude: number;
  longitude: number;
  className?: string;
}) {
  const name = usePlaceName(latitude, longitude);

  return (
    <span className={cn("truncate", className)}>
      {name === "" ? coordinatesOf(latitude, longitude) : name}
    </span>
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

/** How far and how long the routed line has run by the time it reaches a waypoint. */
function progressLabel(preview: PlanRoutePreview | null, index: number): string {
  const at = index === 0 ? undefined : preview?.waypointProgress?.[index];
  if (!at) {
    return "";
  }
  const minutes = Math.round((at.movingSeconds ?? 0) / 60);

  return minutes === 0
    ? formatDistance(at.distanceMetres)
    : `${formatDistance(at.distanceMetres)} · ${formatDuration(minutes * 60)}`;
}

function Pill({ pressed, className, ...rest }: { pressed?: boolean } & ComponentProps<"button">) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      className={cn(
        "inline-flex h-8 items-center gap-1.5 rounded-[9px] border px-2.5 text-xs",
        pressed
          ? "border-transparent bg-[color-mix(in_oklab,var(--accent)_16%,transparent)] text-[var(--accent)]"
          : "border-[var(--rule)] text-[var(--ink)]",
        className,
      )}
      {...rest}
    />
  );
}

/** The card's mark doubles as the plans menu: start a new plan, or delete this one. */
function PlansMenu({
  name,
  saved,
  published,
  deleting,
  deleteError,
  onDelete,
}: {
  name: string;
  saved: boolean;
  published: boolean;
  deleting: boolean;
  deleteError: string | null;
  onDelete: () => void;
}) {
  const [confirming, setConfirming] = useState(false);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <button
              type="button"
              aria-label="Plans"
              className="relative grid size-9 shrink-0 place-items-center rounded-md [background:var(--mark)] text-[var(--mark-ink)]"
            />
          }
        >
          <IconRoute aria-hidden="true" size={18} stroke={1.8} />
          <span className="absolute -right-1 -bottom-1 grid size-4 place-items-center rounded-full bg-[var(--panel)] text-[var(--ink)] shadow-sm ring-1 ring-[var(--rule)]">
            <IconChevronDown aria-hidden="true" size={10} stroke={2.5} />
          </span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-60">
          <DropdownMenuItem render={<Link to="/plan" />}>
            <IconPlus size={14} /> New plan
          </DropdownMenuItem>
          {saved ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onClick={() => setConfirming(true)}>
                <IconTrash size={14} /> Delete this plan…
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <AlertDialog open={confirming} onOpenChange={setConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete “{name}”?</AlertDialogTitle>
            <AlertDialogDescription>
              {published
                ? "It is removed from every rider's Wahoo. This cannot be undone."
                : "It is a draft, so nothing is on Wahoo. This cannot be undone."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError ? (
            <p role="alert" className="text-[var(--alert)] text-sm">
              Could not delete plan: {deleteError}
            </p>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel>Keep it</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={deleting} onClick={onDelete}>
              Delete plan
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

/** A routed leg bends; a straight one is a bare stroke. */
const ROUTED_LEG = "M12 0 C20 4 20 8 12 10 C4 12 4 16 12 20";
const STRAIGHT_LEG = "M12 0 V20";

/** The leg arriving at a waypoint, drawn as the connection between two marks; a click switches it. */
function Leg({ straight, to, onToggle }: { straight: boolean; to: string; onToggle: () => void }) {
  return (
    <button
      type="button"
      aria-pressed={straight}
      onClick={onToggle}
      aria-label={straight ? `Route to ${to} normally` : `Straight line to ${to}`}
      title={straight ? "Straight — click to route" : "Routed — click to draw straight"}
      className="absolute top-[-10px] left-0 z-10 flex h-5 w-6 justify-center text-[color-mix(in_oklab,var(--ink-2)_40%,transparent)] hover:text-[var(--accent)]"
    >
      <svg aria-hidden="true" viewBox="0 0 24 20" className="h-full w-6 overflow-visible">
        <path
          d={straight ? STRAIGHT_LEG : ROUTED_LEG}
          fill="none"
          stroke="currentColor"
          strokeWidth={2.5}
          strokeLinecap="round"
          strokeDasharray={straight ? undefined : "0.1 4"}
        />
      </svg>
    </button>
  );
}

/** An avoided area drawn as its own shape, as the map shows it; only circles exist so far. */
function AreaShape() {
  return (
    <svg aria-hidden="true" viewBox="0 0 14 14" className="size-3.5 shrink-0 text-[var(--alert)]">
      <circle
        cx="7"
        cy="7"
        r="5.5"
        fill="currentColor"
        fillOpacity="0.18"
        stroke="currentColor"
        strokeWidth="1.5"
      />
    </svg>
  );
}

/** Avoided areas sit on the footer as one line and open upwards, leaving the list its room. */
function AvoidDrawer({
  areas,
  onDelete,
}: {
  areas: PlannerAvoid[];
  onDelete: (id: number) => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <div className="mx-5 mb-1 flex shrink-0 flex-col rounded-lg bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] px-3">
      {open ? (
        <ul
          aria-label="Avoided areas"
          className="flex max-h-40 flex-col gap-1 overflow-y-auto pt-2"
        >
          {areas.map((area) => (
            <li key={area.id} className="flex items-center gap-2 text-xs">
              <AreaShape />
              <PlaceName
                latitude={area.latitude}
                longitude={area.longitude}
                className="min-w-0 flex-1 text-[var(--ink-2)]"
              />
              <span className="shrink-0 tabular-nums">{formatDistance(area.radiusMetres)}</span>
              <Button
                variant="ghost"
                icon={<IconTrash size={14} />}
                aria-label="Delete avoided area"
                onClick={() => onDelete(area.id)}
              />
            </li>
          ))}
        </ul>
      ) : null}
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 py-2 text-left text-sm"
      >
        <span className="flex-1 font-medium">Avoided areas</span>
        <span className="text-[var(--ink-2)] text-xs">{formatCount(areas.length, "area")}</span>
        <IconChevronDown
          aria-hidden="true"
          size={14}
          className={cn(
            "shrink-0 text-[var(--ink-2)] transition-transform",
            open ? "" : "rotate-180",
          )}
        />
      </button>
    </div>
  );
}

function RouteFigures({ preview }: { preview: PlanRoutePreview }) {
  const figure = (icon: typeof IconRoute, label: string, value: string) => {
    const Icon = icon;
    return (
      <div>
        <dt className="flex items-center justify-center gap-1 text-[10px] text-[var(--ink-2)] uppercase">
          <Icon aria-hidden="true" size={11} /> {label}
        </dt>
        <dd className="font-semibold text-sm tabular-nums">{value}</dd>
      </div>
    );
  };

  return (
    <dl className="mb-3 grid grid-cols-3 gap-2 rounded-lg bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] px-3 py-2 text-center">
      {figure(IconRoute, "Distance", formatDistance(preview.distanceMetres))}
      {figure(IconMountain, "Climb", formatAscent(preview.ascentMetres))}
      {figure(
        IconClock,
        "Time",
        formatDuration(
          preview.movingSeconds === undefined
            ? undefined
            : Math.round(preview.movingSeconds / 60) * 60,
        ),
      )}
    </dl>
  );
}

/** Short plans show every waypoint; only a longer one folds. */
const COLLAPSE_ABOVE = 7;

type Segment = { kind: "row"; index: number } | { kind: "gap"; from: number; to: number };

/** Rows to show and the hidden runs between them, in order. */
function segments(count: number, shown: (index: number) => boolean): Segment[] {
  const out: Segment[] = [];
  for (let index = 0; index < count; index++) {
    const last = out.at(-1);
    if (shown(index)) {
      out.push({ kind: "row", index });
    } else if (last?.kind === "gap") {
      last.to = index;
    } else {
      out.push({ kind: "gap", from: index, to: index });
    }
  }
  return out;
}

/** A run of hidden waypoints, by index; the map lights the line from the one before to the one after. */
export interface HiddenRun {
  from: number;
  to: number;
}

export interface PlannerSidebarProps {
  state: PlannerState;
  preview: PlanRoutePreview | null;
  planId: number | null;
  /** Whether the stored plan is published; the footer's switch only takes effect on save. */
  published: boolean;
  /** Whether the plan differs from what is stored, the publication aside. */
  changed: boolean;
  saving: boolean;
  deleting?: boolean;
  deleteError?: string | null;
  saveError: string | null;
  /** How many turn instructions the routing engine gave the loaded/saved plan's line. */
  turnCount?: number;
  /** The waypoint last touched or hovered on the map. */
  focusId?: number | null;
  /** The waypoints `preview` was routed for; a row it no longer describes shows no distance. */
  routedFor?: PlannerState["waypoints"] | null;
  onSave: (published: boolean) => void;
  onDelete?: () => void;
  onHighlight?: (run: HiddenRun | null) => void;
  dispatch: Dispatch<Parameters<typeof plannerReducer>[1]>;
}

/** The planner's column beside the map; the map remains visible while the list changes. */
export function PlannerSidebar({
  state,
  preview,
  planId,
  published,
  changed,
  saving,
  deleting = false,
  deleteError = null,
  saveError,
  turnCount,
  focusId = null,
  routedFor,
  onSave,
  onDelete = () => {},
  onHighlight = () => {},
  dispatch,
}: PlannerSidebarProps) {
  const [publish, setPublish] = useState(published);
  const [storedPublished, setStoredPublished] = useState(published);
  if (storedPublished !== published) {
    setStoredPublished(published);
    setPublish(published);
  }
  const count = state.waypoints.length;
  const focus = state.waypoints.findIndex((waypoint) => waypoint.id === focusId);
  // A new focus folds whatever was opened, so the list follows the work on the map.
  const [opened, setOpened] = useState<ReadonlySet<number>>(() => new Set());
  const [openedFor, setOpenedFor] = useState(focusId);
  if (openedFor !== focusId) {
    setOpenedFor(focusId);
    setOpened(new Set());
  }

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
  const scroller = useRef<HTMLDivElement>(null);
  const speed = useRef(0);
  const frame = useRef<number | null>(null);
  const step = () => {
    if (!scroller.current || speed.current === 0) {
      frame.current = null;

      return;
    }
    scroller.current.scrollTop += speed.current;
    frame.current = requestAnimationFrame(step);
  };
  const scrollNearEdge = (pointerY: number) => {
    if (!scroller.current) {
      return;
    }
    const box = scroller.current.getBoundingClientRect();
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

  // The ends and the waypoint last touched on the map with its neighbours stay; a drag shows all.
  const folding = count > COLLAPSE_ABOVE && visualOrder === null;
  const shown = (index: number) =>
    !folding ||
    index === 0 ||
    index === count - 1 ||
    (focus >= 0 && Math.abs(index - focus) <= 1) ||
    opened.has(state.waypoints[index]?.id ?? -1);
  // Undefined `routedFor` trusts the preview as it stands, as a story or a stored plan does.
  const routed = (index: number) => {
    const at = routedFor?.[index];
    const waypoint = state.waypoints[index];
    return (
      routedFor === undefined ||
      (at?.id === waypoint?.id &&
        at?.longitude === waypoint?.longitude &&
        at?.latitude === waypoint?.latitude)
    );
  };
  const progress = (index: number) =>
    routed(index) ? (preview?.waypointProgress?.[index]?.distanceMetres ?? Number.NaN) : Number.NaN;
  const canSave = state.name.trim() !== "" && count >= 2;
  const edited = changed || publish !== published;

  return (
    <section className="flex min-h-0 flex-1 flex-col bg-[var(--base)] p-3">
      <div className="flex min-h-0 flex-1 flex-col rounded-2xl bg-[var(--panel)] shadow-[var(--shadow)]">
        <div className="flex shrink-0 items-center gap-2.5 px-5 pt-5 pb-3">
          <PlansMenu
            name={state.name}
            saved={planId !== null}
            published={published}
            deleting={deleting}
            deleteError={deleteError}
            onDelete={onDelete}
          />
          <label className="flex min-w-0 flex-1 items-center gap-1.5 rounded-lg border border-[var(--rule)] bg-[var(--base)] px-2.5 py-1.5 focus-within:border-[var(--accent)] hover:border-[color-mix(in_oklab,var(--ink-2)_40%,var(--rule))]">
            <input
              aria-label="Plan name"
              value={state.name}
              maxLength={120}
              onChange={(event) => dispatch({ type: "setName", name: event.target.value })}
              placeholder="Name this route"
              className="min-w-0 flex-1 truncate bg-transparent font-semibold text-base outline-none placeholder:font-normal placeholder:text-[var(--ink-2)]"
            />
            <IconPencil aria-hidden="true" size={14} className="shrink-0 text-[var(--ink-2)]" />
          </label>
          <PlanDeliveryTrigger planId={planId} published={published} />
        </div>
        <div
          ref={scroller}
          className="flex min-h-0 flex-1 flex-col overflow-y-auto px-5"
          onDragOver={(event) => scrollNearEdge(event.clientY)}
          onDragLeave={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
              stopScrolling();
            }
          }}
        >
          {preview && count > 1 ? <RouteFigures preview={preview} /> : null}
          <div className="flex flex-wrap gap-1.5 pb-3">
            <DropdownMenu>
              <DropdownMenuTrigger render={<Pill aria-label="Route type" />}>
                <IconRoute2 aria-hidden="true" size={13} />
                {PROFILES.find((option) => option.key === state.profile)?.label}
                <IconChevronDown aria-hidden="true" size={12} />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                {PROFILES.map((option) => (
                  <DropdownMenuItem
                    key={option.key}
                    onClick={() => dispatch({ type: "setProfile", profile: option.key })}
                  >
                    {option.label}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
            <Pill
              pressed={state.cues}
              title="Turn cues on Wahoo"
              onClick={() => dispatch({ type: "setCues", cues: !state.cues })}
            >
              Turn cues
              {state.cues && turnCount !== undefined ? ` · ${turnCount}` : ""}
            </Pill>
          </div>
          {count === 0 ? (
            <p className="py-6 text-center text-[var(--ink-2)] text-sm">
              Click the map to start the route.
            </p>
          ) : null}
          <ol
            aria-label="Waypoints"
            data-dragging={visualOrder === null ? undefined : true}
            className="flex flex-col pb-2 empty:hidden"
          >
            {segments(count, shown).map((segment, position, all) => {
              if (segment.kind === "gap") {
                const run = { from: segment.from, to: segment.to };
                const straight = state.waypoints
                  .slice(segment.from, segment.to + 2)
                  .filter((waypoint) => waypoint.straight).length;
                const metres = progress(segment.to + 1) - progress(segment.from - 1);
                return (
                  <li key={`gap-${segment.from}`} className="relative flex h-8 items-center">
                    <span
                      aria-hidden="true"
                      className="absolute top-[-10px] bottom-[-10px] left-[11px] border-[color-mix(in_oklab,var(--ink-2)_35%,transparent)] border-l-2 border-dotted"
                    />
                    <button
                      type="button"
                      onClick={() => {
                        onHighlight(null);
                        setOpened((current) => {
                          const next = new Set(current);
                          for (const waypoint of state.waypoints.slice(run.from, run.to + 1)) {
                            next.add(waypoint.id);
                          }
                          return next;
                        });
                      }}
                      onMouseEnter={() => onHighlight(run)}
                      onMouseLeave={() => onHighlight(null)}
                      onFocus={() => onHighlight(run)}
                      onBlur={() => onHighlight(null)}
                      className="ml-[34px] flex min-w-0 flex-1 items-center gap-1.5 rounded-md px-2 py-1 text-left text-[var(--ink-2)] text-xs hover:bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]"
                    >
                      <IconChevronDown aria-hidden="true" size={12} />
                      {formatCount(segment.to - segment.from + 1, "more waypoint")}
                      {Number.isFinite(metres) ? ` · ${formatDistance(metres)}` : ""}
                      {straight > 0 ? ` · ${straight} straight` : ""}
                    </button>
                  </li>
                );
              }
              const index = segment.index;
              const waypoint = visualWaypoints[index];
              if (!waypoint) {
                return null;
              }
              const stateIndex = state.waypoints.indexOf(waypoint);
              const label = waypointLabel(index, count);
              const focused = waypoint.id === focusId;
              // The leg into a row after a hidden run starts at a hidden waypoint; the run's line stands for it.
              const afterGap = all[position - 1]?.kind === "gap";

              return (
                <li
                  key={waypoint.id}
                  data-waypoint-id={waypoint.id}
                  className="relative flex h-11 shrink-0 items-center gap-1"
                >
                  {index > 0 && !afterGap ? (
                    <Leg
                      straight={Boolean(waypoint.straight)}
                      to={label}
                      onToggle={() =>
                        dispatch({
                          type: "setStraight",
                          id: waypoint.id,
                          straight: !waypoint.straight,
                        })
                      }
                    />
                  ) : null}
                  <div
                    role="group"
                    draggable
                    aria-label={`Drag ${label} to reorder`}
                    className={cn(
                      "flex min-w-0 flex-1 items-center gap-2.5",
                      dragging.current === waypoint.id ? "opacity-60" : undefined,
                    )}
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
                      data-focused={focused ? true : undefined}
                      className={cn(
                        "relative z-10 grid size-6 shrink-0 place-items-center rounded-full font-semibold text-white text-xs ring-2 ring-[var(--panel)]",
                        index === 0 || index === count - 1
                          ? "bg-[var(--accent)]"
                          : "bg-[var(--ink-2)]",
                        focused ? "outline-2 outline-[var(--accent)] outline-offset-2" : undefined,
                      )}
                    >
                      <WaypointMarker index={index} count={count} />
                    </span>
                    <span className="flex min-w-0 flex-1 flex-col">
                      <PlaceName
                        latitude={waypoint.latitude}
                        longitude={waypoint.longitude}
                        className={cn("text-sm", focused ? "font-semibold" : undefined)}
                      />
                      {!routed(stateIndex) || progressLabel(preview, stateIndex) === "" ? null : (
                        <span className="truncate text-[var(--ink-2)] text-xs tabular-nums">
                          {progressLabel(preview, stateIndex)}
                        </span>
                      )}
                    </span>
                    <Button
                      variant="ghost"
                      className="sr-only focus:not-sr-only"
                      aria-label={`Move ${label} up`}
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
                      aria-label={`Move ${label} down`}
                      disabled={index === count - 1}
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
        </div>
        {state.avoid.length > 0 ? (
          <AvoidDrawer
            areas={state.avoid}
            onDelete={(id) => dispatch({ type: "deleteAvoid", id })}
          />
        ) : null}
        <div className="flex shrink-0 flex-col gap-2 p-3">
          <div className="flex items-center gap-2">
            <Segmented
              label="Publication"
              size="sm"
              items={[
                { key: "draft", label: "Draft", fill: "var(--hold)" },
                { key: "published", label: "Published", fill: "var(--good)" },
              ]}
              value={publish ? "published" : "draft"}
              onChange={(next) => setPublish(next === "published")}
            />
            <Button
              variant={edited && canSave ? "default" : "outline"}
              className="flex-1"
              disabled={saving || !edited || !canSave}
              onClick={() => onSave(publish)}
            >
              {saving ? "Saving…" : "Save"}
            </Button>
          </div>
          {saveError ? (
            <p role="alert" className="text-[var(--alert)] text-xs">
              Could not save plan: {saveError}
            </p>
          ) : null}
        </div>
      </div>
    </section>
  );
}
