/**
 * Choosing a stretch of route by dragging the map.
 *
 * The chart already answers "show me this climb" to a drag across it, and the
 * painted route is the other half of the same instrument: a reader looking at
 * the map has the climb in front of them there, and asking for it by pointing
 * at the chart underneath is a translation nobody should have to make. So a
 * drag that begins on the line picks the ground between where it began and
 * where it ended, in the same distance window the chart hands over.
 *
 * Nearness to the line is what tells the two gestures apart. A drag that begins
 * on the route selects; a drag that begins off it is left entirely alone and
 * pans the map, which is the ordinary way of looking around and must not become
 * a mode. There is no timer anywhere in here: what a gesture meant is decided by
 * where it started and how far it travelled, both of which are known the moment
 * they happen, and neither of which behaves differently for a slow hand.
 *
 * The map instance arrives through a small structural interface rather than as
 * MapLibre's `Map`, because everything this needs of it — projecting a position,
 * unprojecting a pixel, the element the pointer lands on, and the pan handler to
 * stand down — is a handful of methods a test can supply without a GPU.
 */

import type { DistanceWindow, Profile } from "./profile";
import { nearestSample } from "./profile";
import { MIN_DRAG_PIXELS, NEAR_ROUTE_PIXELS, spanBetween, widened } from "./selection";

/** As much of a map as picking a stretch off it needs. */
export interface SelectableMap {
  getCanvasContainer(): HTMLElement;
  project(lngLat: [number, number]): { x: number; y: number };
  unproject(point: [number, number]): { lng: number; lat: number };
  dragPan: { enable(): void; disable(): void; isEnabled(): boolean };
}

export interface RouteSelectionOptions {
  /**
   * The whole route's profile, which is what turns a pixel into a distance. A
   * windowed one would describe a stretch the drag is allowed to leave.
   */
  profile: Profile;
  /** The stretch under the hand while it is still being chosen, for the paint. */
  onPending: (window: DistanceWindow | null) => void;
  /** The stretch settled on, already grown to the shortest window allowed. */
  onSelect: (window: DistanceWindow) => void;
}

/** The cursor while a stretch is being drawn, so the map stops offering a pan. */
const SELECTING_CURSOR = "crosshair";

interface Drag {
  pointerId: number;
  originX: number;
  originY: number;
  anchorMetres: number;
  /**
   * The last position the pointer was near the route at.
   *
   * A hand that wanders off the line mid-drag has not asked for the far side of
   * a hairpin it happens to pass nearer to; it has simply left the road. The
   * selection stays where it last agreed with the route, so overshooting the end
   * of a climb settles on the end of the climb.
   */
  lastMetres: number;
  /** Whether the hand has travelled far enough to mean a range at all. */
  moved: boolean;
  /** Whether the pan handler was ours to stand down, and so ours to restore. */
  restorePan: boolean;
}

/**
 * Where along the route a pointer is, or null when it is nowhere near it.
 *
 * Nearness is judged in pixels against the projected sample rather than in
 * metres on the ground, because the hit area a hand aims at is the drawn line,
 * whose width on screen has nothing to do with the scale the map is at.
 */
function nearestMetres(
  map: SelectableMap,
  profile: Profile,
  clientX: number,
  clientY: number,
): number | null {
  const bounds = map.getCanvasContainer().getBoundingClientRect();
  const x = clientX - bounds.left;
  const y = clientY - bounds.top;
  const { lng, lat } = map.unproject([x, y]);
  const index = nearestSample(profile, lng, lat);
  const sample = index === null ? undefined : profile.samples[index];
  if (!sample) {
    return null;
  }
  const projected = map.project([sample.longitude, sample.latitude]);

  return Math.hypot(projected.x - x, projected.y - y) <= NEAR_ROUTE_PIXELS
    ? sample.distanceMetres
    : null;
}

/**
 * Lets a drag across the route pick the stretch it covers. Returns the way to
 * take the gesture back off the map.
 *
 * The pointer is followed on the window rather than on the map's own element:
 * a selection that stopped at the edge of the canvas would end wherever the
 * hand happened to leave it, and a reader dragging to the end of a climb tends
 * to overshoot the pane. Pointer capture would do the same job for a pointer
 * that is still down, but not for the release that arrives after the hand has
 * left, and one mechanism that covers both is fewer to reason about.
 */
export function routeSelection(map: SelectableMap, options: RouteSelectionOptions): () => void {
  const container = map.getCanvasContainer();
  const previousCursor = container.style.cursor;
  let drag: Drag | null = null;
  let reported: DistanceWindow | null = null;

  /**
   * Reports the stretch under the hand, and only when it has changed.
   *
   * A gesture settles on samples of the profile, so a hand crossing a few
   * pixels of a long route keeps arriving at the same stretch — and every
   * report of one repaints the whole route. Saying nothing when nothing has
   * changed keeps a drag across a ninety kilometre stage as cheap as a drag
   * across a short one.
   */
  const report = (next: DistanceWindow | null) => {
    const same =
      next === reported ||
      (next !== null &&
        reported !== null &&
        next.startMetres === reported.startMetres &&
        next.endMetres === reported.endMetres);
    if (same) {
      return;
    }
    reported = next;
    options.onPending(next);
  };

  const release = () => {
    const started = drag;
    drag = null;
    if (!started) {
      return;
    }
    if (started.restorePan) {
      map.dragPan.enable();
    }
    container.style.cursor = previousCursor;
    report(null);
  };

  const onPointerDown = (event: PointerEvent) => {
    // A right-click opens a menu and a second finger is exploration; neither is
    // a selection, and both must hand the map back rather than leave a range
    // half-drawn behind them.
    if (!event.isPrimary || event.button !== 0) {
      release();

      return;
    }
    const metres = nearestMetres(map, options.profile, event.clientX, event.clientY);
    if (metres === null) {
      // Away from the route: this drag belongs to the map, which pans as it
      // always has. Nothing here has touched it.
      return;
    }
    // Stood down before MapLibre hears the gesture at all: a pointer event
    // precedes the mouse or touch event the pan handler starts from, so the pan
    // never begins and there is no movement to undo.
    const restorePan = map.dragPan.isEnabled();
    if (restorePan) {
      map.dragPan.disable();
    }
    drag = {
      pointerId: event.pointerId,
      originX: event.clientX,
      originY: event.clientY,
      anchorMetres: metres,
      lastMetres: metres,
      moved: false,
      restorePan,
    };
    container.style.cursor = SELECTING_CURSOR;
  };

  const onPointerMove = (event: PointerEvent) => {
    const started = drag;
    if (!started || event.pointerId !== started.pointerId) {
      return;
    }
    const metres = nearestMetres(map, options.profile, event.clientX, event.clientY);
    if (metres !== null) {
      started.lastMetres = metres;
    }
    if (
      !started.moved &&
      Math.hypot(event.clientX - started.originX, event.clientY - started.originY) < MIN_DRAG_PIXELS
    ) {
      return;
    }
    started.moved = true;
    report(spanBetween(started.anchorMetres, started.lastMetres));
  };

  const onPointerUp = (event: PointerEvent) => {
    const started = drag;
    if (!started || event.pointerId !== started.pointerId) {
      return;
    }
    // A hand that barely moved was pointing at the route, not drawing across it.
    const chosen = started.moved ? spanBetween(started.anchorMetres, started.lastMetres) : null;
    release();
    if (chosen) {
      options.onSelect(widened(chosen, options.profile.totalDistanceMetres));
    }
  };

  const onPointerCancel = (event: PointerEvent) => {
    if (drag && event.pointerId === drag.pointerId) {
      release();
    }
  };

  /**
   * Escape abandons the stretch being drawn.
   *
   * Taken in the capture phase and marked handled, so the same key that returns
   * a zoomed map to the whole route does not do both at once: a reader half way
   * through drawing a window is asking to stop drawing it, not to throw away the
   * view they were drawing it on.
   */
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === "Escape" && drag) {
      event.preventDefault();
      release();
    }
  };

  container.addEventListener("pointerdown", onPointerDown);
  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", onPointerUp);
  window.addEventListener("pointercancel", onPointerCancel);
  document.addEventListener("keydown", onKeyDown, true);

  return () => {
    release();
    container.removeEventListener("pointerdown", onPointerDown);
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    window.removeEventListener("pointercancel", onPointerCancel);
    document.removeEventListener("keydown", onKeyDown, true);
  };
}
