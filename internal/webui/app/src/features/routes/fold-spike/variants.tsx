/**
 * Five ways the dock's own fold control could replace the top-edge pill.
 *
 * The pill sits centred on the seam whether the dock is open or shut, and the
 * user dislikes it there. Each variant below moves the control somewhere else
 * and takes its own position on what the folded state looks like. Storybook
 * only: nothing here is imported by the app.
 */

import {
  IconChevronDown,
  IconChevronUp,
  IconCloud,
  IconLayoutBottombarCollapse,
  IconLayoutBottombarExpand,
  IconMountain,
} from "@tabler/icons-react";
import { type ReactNode, useState } from "react";
import type { Highlight } from "../../../lib/highlight";
import type { MeasureKey } from "../../../lib/measures";
import type { DistanceWindow } from "../../../lib/profile";
import {
  climbs,
  coordinates,
  profile,
  rideStart,
  route,
  surface,
  weatherSamples,
} from "../../../storybook/fixtures";
import { RouteDock, type RouteDockProps } from "../RouteDock";

/** `14:20`, in the reader's own zone — matches `RouteDock`'s own formatting. */
function clockAt(at: Date): string {
  return at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

const BACK = weatherSamples[weatherSamples.length - 1]?.arrivalAt;

const CONTROL =
  "text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]";

/** Everything `RouteDock` needs beyond `open`/`onOpenChange`, wired once per variant. */
function useDockProps(): Omit<RouteDockProps, "open" | "onOpenChange"> {
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  const [measure, setMeasure] = useState<MeasureKey | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  const [startAt, setStartAt] = useState<Date | null>(new Date(rideStart.getTime() - 60 * 60_000));

  return {
    title: route.title,
    profile,
    distanceMetres: route.distanceMetres,
    ascentMetres: route.ascentMetres,
    surface,
    climbs,
    onSelectClimb: () => {},
    coordinates,
    samples: weatherSamples,
    startAt,
    onStartAtChange: setStartAt,
    movingSeconds: route.movingSeconds,
    activeMetres,
    onActiveChange: setActiveMetres,
    zoomWindow,
    onZoomChange: setZoomWindow,
    highlight,
    onHighlightChange: setHighlight,
    measure,
    onMeasureChange: setMeasure,
    unitSystem: "metric",
  };
}

/** ~120px of "map" above the dock, so the fold seam reads against something. */
function Stage({ children }: { children: ReactNode }) {
  return (
    <div className="bg-[var(--ground)] p-6">
      <div className="relative">
        <div className="h-[120px] rounded-t-lg bg-[var(--base)]" aria-hidden="true" />
        {children}
      </div>
    </div>
  );
}

/** Hides the dock's own top-edge handle; a real change would add a `hideHandle` prop instead. */
const HIDE_HANDLE = "[&_button[aria-label='Hide_the_route_detail']]:hidden";
/** Hides the dock's own folded pill; a real change would add a `hideHandle` prop instead. */
const HIDE_PILL = "[&>button[aria-expanded=false]]:hidden";

/** A · Corner — a small square button in the panel's top-right, alone on the map's foot when folded. */
export function CornerFold() {
  const [open, setOpen] = useState(true);
  const dockProps = useDockProps();

  return (
    <Stage>
      <div className={`relative ${HIDE_HANDLE} ${HIDE_PILL}`}>
        <RouteDock {...dockProps} open={open} onOpenChange={setOpen} />
        {open ? (
          <button
            type="button"
            aria-expanded
            aria-label="Collapse the route detail"
            onClick={() => setOpen(false)}
            className={`absolute top-4 right-4 flex size-7 items-center justify-center rounded-md ${CONTROL}`}
          >
            <IconChevronDown size={28} stroke={1.5} aria-hidden="true" />
          </button>
        ) : null}
      </div>
      {open ? null : (
        <button
          type="button"
          aria-expanded={false}
          aria-label="Show the route detail"
          onClick={() => setOpen(true)}
          className={`absolute top-[86px] right-6 flex size-7 items-center justify-center rounded-md ${CONTROL}`}
        >
          <IconChevronUp size={28} stroke={1.5} aria-hidden="true" />
        </button>
      )}
    </Stage>
  );
}

const RAIL_STOP =
  "flex w-14 flex-col items-center gap-0.5 rounded-md px-1 py-1.5 text-[10px] leading-none";

/** B · Rail stop — the fold is a third rail item; folded, the rail lies flat along the map's foot. */
export function RailFold() {
  const [open, setOpen] = useState(true);
  const dockProps = useDockProps();

  return (
    <Stage>
      <div className={`relative ${HIDE_HANDLE} ${HIDE_PILL}`}>
        <RouteDock {...dockProps} open={open} onOpenChange={setOpen} />
        {open ? (
          <button
            type="button"
            aria-expanded
            aria-label="Collapse the route detail"
            onClick={() => setOpen(false)}
            className={`absolute top-[108px] left-6 ${RAIL_STOP} ${CONTROL}`}
          >
            <IconLayoutBottombarCollapse size={15} stroke={2} aria-hidden="true" />
            Hide
          </button>
        ) : null}
      </div>
      {open ? null : (
        <div className="absolute inset-x-6 top-[72px] flex items-center gap-2 rounded-lg bg-[var(--panel)] p-1.5 shadow-[var(--shadow)] ring-1 ring-black/5">
          <button
            type="button"
            aria-expanded={false}
            aria-label="Show the profile"
            onClick={() => setOpen(true)}
            className={`flex-row gap-1.5 rounded-md px-2 py-1 text-xs ${CONTROL}`}
          >
            <IconMountain
              size={15}
              stroke={2}
              aria-hidden="true"
              className="inline-block align-text-bottom"
            />{" "}
            Profile
          </button>
          <button
            type="button"
            aria-expanded={false}
            aria-label="Show the forecast"
            onClick={() => setOpen(true)}
            className={`flex-row gap-1.5 rounded-md px-2 py-1 text-xs ${CONTROL}`}
          >
            <IconCloud
              size={15}
              stroke={2}
              aria-hidden="true"
              className="inline-block align-text-bottom"
            />{" "}
            Forecast
          </button>
          <button
            type="button"
            aria-expanded={false}
            aria-label="Show the route detail"
            onClick={() => setOpen(true)}
            className={`ml-auto flex-row gap-1.5 rounded-md px-2 py-1 text-xs ${CONTROL}`}
          >
            <IconLayoutBottombarExpand
              size={15}
              stroke={2}
              aria-hidden="true"
              className="inline-block align-text-bottom"
            />{" "}
            Show
          </button>
        </div>
      )}
    </Stage>
  );
}

/** C · Grip — a full-width drag-style grip on the seam; folded, it sits on the map's foot with the return time. */
export function GripFold() {
  const [open, setOpen] = useState(true);
  const dockProps = useDockProps();

  return (
    <Stage>
      <div className={`relative ${HIDE_HANDLE} ${HIDE_PILL}`}>
        <RouteDock {...dockProps} open={open} onOpenChange={setOpen} />
        {open ? (
          <button
            type="button"
            aria-expanded
            aria-label="Collapse the route detail"
            onClick={() => setOpen(false)}
            className={`group absolute inset-x-6 -top-1 flex h-3 items-center justify-center ${CONTROL}`}
          >
            <IconChevronUp
              size={13}
              stroke={2}
              aria-hidden="true"
              className="absolute -top-3.5 opacity-0 transition-opacity group-hover:opacity-100"
            />
            <span className="h-1 w-full rounded-full bg-[var(--rule)]" />
          </button>
        ) : null}
      </div>
      {open ? null : (
        <div className="absolute inset-x-6 top-[104px] flex items-center gap-3">
          <button
            type="button"
            aria-expanded={false}
            aria-label="Show the route detail"
            onClick={() => setOpen(true)}
            className={`h-3 w-24 rounded-full bg-[var(--rule)] ${CONTROL}`}
          />
          {BACK === undefined ? null : (
            <span className="text-[10px] text-[var(--ink-2)]">back {clockAt(BACK)}</span>
          )}
        </div>
      )}
    </Stage>
  );
}

/** D · In the line — an icon button beside the profile stop's ⓘ; the folded pill is unchanged. */
export function InlineFold() {
  const [open, setOpen] = useState(true);
  const dockProps = useDockProps();

  return (
    <Stage>
      <div className={`relative ${HIDE_HANDLE}`}>
        <RouteDock {...dockProps} open={open} onOpenChange={setOpen} />
        {open ? (
          <button
            type="button"
            aria-expanded
            aria-label="Collapse the route detail"
            onClick={() => setOpen(false)}
            className={`absolute top-4 right-96 rounded-md p-0.5 ${CONTROL}`}
          >
            <IconLayoutBottombarCollapse size={16} stroke={1.8} aria-hidden="true" />
          </button>
        ) : null}
      </div>
    </Stage>
  );
}

/** E · Tab — a short tab hanging from the seam at the left; folded, it rises from the map's foot instead. */
export function TabFold() {
  const [open, setOpen] = useState(true);
  const dockProps = useDockProps();

  return (
    <Stage>
      <div className={`relative ${HIDE_HANDLE} ${HIDE_PILL}`}>
        <RouteDock {...dockProps} open={open} onOpenChange={setOpen} />
        {open ? (
          <button
            type="button"
            aria-expanded
            aria-label="Collapse the route detail"
            onClick={() => setOpen(false)}
            className={`absolute top-0 left-4 flex h-6 w-28 items-center justify-center gap-1 rounded-b-md bg-[var(--base)] text-xs ${CONTROL}`}
          >
            Hide
            <IconChevronUp size={13} stroke={2} aria-hidden="true" />
          </button>
        ) : null}
      </div>
      {open ? null : (
        <button
          type="button"
          aria-expanded={false}
          aria-label="Show the route detail"
          onClick={() => setOpen(true)}
          className={`absolute top-[96px] left-6 flex h-6 items-center justify-center gap-1 whitespace-nowrap rounded-t-md bg-[var(--panel)] px-3 text-xs shadow-[var(--shadow)] ring-1 ring-black/5 ${CONTROL}`}
        >
          {BACK === undefined ? "Profile & forecast" : `Profile & forecast · back ${clockAt(BACK)}`}
          <IconChevronDown size={13} stroke={2} aria-hidden="true" />
        </button>
      )}
    </Stage>
  );
}
