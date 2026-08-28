import { useEffect, useRef, useState } from "react";
import { Marker, useMap } from "react-map-gl/maplibre";
import { formatDistance, formatElevation } from "../../lib/format";
import type { ProfileSample } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import { SURFACE_STYLES, surfaceKindAt } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";

type TooltipAnchor = "top-left" | "top-right" | "bottom-left" | "bottom-right";

/** The gap kept between the position dot and the tooltip that labels it. */
const TOOLTIP_GAP_PIXELS = 14;

/**
 * A reasonable guess at the tooltip's box before its first real measurement.
 * Matches the CSS `max-width` and roughly one line of `--type-small` text.
 */
const DEFAULT_TOOLTIP_SIZE = { width: 220, height: 32 };

/**
 * Whether the box opens away from the point (`"start"`) or back over it
 * (`"end"`), chosen by which side actually has room for it — falling back to
 * whichever side has more room when neither fits, on a pane too narrow for
 * the box either way.
 */
function fittingSide(pointCoordinate: number, size: number, extent: number): "start" | "end" {
  const needed = size + TOOLTIP_GAP_PIXELS;
  const forwardRoom = extent - pointCoordinate;
  if (forwardRoom >= needed) {
    return "start";
  }
  if (pointCoordinate >= needed) {
    return "end";
  }

  return forwardRoom >= pointCoordinate ? "start" : "end";
}

/**
 * Which corner of the tooltip sits on the point, chosen from the room the box
 * actually has on each side rather than from a fixed midpoint — a point past
 * the middle of the pane can still have plenty of room ahead of it, and a
 * midpoint split would flip the box away from the map's edge only by chance.
 */
function tooltipAnchor(
  point: { x: number; y: number },
  container: { clientWidth: number; clientHeight: number },
  size: { width: number; height: number },
): TooltipAnchor {
  const horizontal =
    fittingSide(point.x, size.width, container.clientWidth) === "start" ? "left" : "right";
  const vertical =
    fittingSide(point.y, size.height, container.clientHeight) === "start" ? "top" : "bottom";

  return `${vertical}-${horizontal}`;
}

/** Nudges the tooltip further along the direction its anchor already opens in. */
function tooltipOffset(anchor: TooltipAnchor): [number, number] {
  return [
    anchor.endsWith("left") ? TOOLTIP_GAP_PIXELS : -TOOLTIP_GAP_PIXELS,
    anchor.startsWith("top") ? TOOLTIP_GAP_PIXELS : -TOOLTIP_GAP_PIXELS,
  ];
}

/**
 * `formatDistance`, but honest about a true zero.
 *
 * The shared formatter reads zero as "nothing to report" and prints `—`,
 * which is right for a stage with no climbing and wrong at the finish of a
 * ride, where zero metres to go is exactly the fact this line exists to
 * state.
 */
function tooltipDistance(metres: number, system: UnitSystem): string {
  return metres > 0 ? formatDistance(metres, system) : system === "imperial" ? "0 ft" : "0 m";
}

/**
 * The numbers for the position under the pointer, on the dot itself.
 *
 * The profile readout below the map says the same thing, but it is inside the
 * `<details>` a reader can and does collapse to give the map the whole pane —
 * and even open, reading it means looking away from the point being asked
 * about.
 *
 * Collapsing the profile also unmounts the readout's own `aria-live` region,
 * so `announce` says whether this tooltip has to carry that announcement
 * itself. Open, it stays `aria-hidden`: the readout already says the same
 * position, and a second live region would announce it twice.
 *
 * A child of the map because placement is judged in screen pixels against the
 * live camera, the same reason `HoverLink` is one — and because the anchor has
 * to answer again whenever the camera moves the point into a different corner
 * of the pane, not only when a fresh hover picks a new one. Pointer events are
 * switched off on the marker itself: it floats over the very line the pointer
 * is following, and a tooltip that intercepted the mouse would freeze the
 * position it labels the moment the cursor drifted under it.
 */
export function PositionTooltip({
  position,
  content,
  endMetres,
  surfaceSummary,
  announce,
  darkBasemap,
  unitSystem,
}: {
  /**
   * Where the marker sits: always the whole-route sample, the same one the
   * dot itself is drawn from. A windowed profile interpolates its own
   * coordinates independently, and on a bend those can differ from the
   * whole route's by enough to put the tooltip beside the dot it is meant
   * to label rather than on it.
   */
  position: ProfileSample;
  /** What the marker says: see `activeProfile` on `RouteOverlay` for which profile this comes from. */
  content: ProfileSample;
  /** The whole route's length, for the distance still left to ride. */
  endMetres: number;
  surfaceSummary: SurfaceSummary | null | undefined;
  announce: boolean;
  /** Whether the ground under this is dark, which is what its two colours follow. */
  darkBasemap: boolean;
  unitSystem: UnitSystem;
}) {
  const { current: map } = useMap();
  const boxRef = useRef<HTMLParagraphElement | null>(null);
  const [size, setSize] = useState(DEFAULT_TOOLTIP_SIZE);
  // Read nowhere: setting it is what forces the anchor below to be recomputed
  // against the camera's current position rather than the one as of the last
  // hover event.
  const [, bumpOnCameraMove] = useState(0);

  const anchor = map
    ? tooltipAnchor(map.project([position.longitude, position.latitude]), map.getContainer(), size)
    : null;

  // Re-attached whenever the anchor changes rather than only once: the marker
  // below is keyed on it and remounts its whole subtree when it does, which
  // leaves this observing a `<p>` that just left the document.
  // biome-ignore lint/correctness/useExhaustiveDependencies: anchor is read nowhere in the body; it is the remount signal, not a value the effect needs.
  useEffect(() => {
    const element = boxRef.current;
    if (!element || typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(() => {
      setSize({ width: element.offsetWidth, height: element.offsetHeight });
    });
    observer.observe(element);

    return () => observer.disconnect();
  }, [anchor]);

  useEffect(() => {
    if (!map) {
      return;
    }
    const onCamera = () => bumpOnCameraMove((tick) => tick + 1);
    map.on("move", onCamera);
    map.on("resize", onCamera);

    return () => {
      map.off("move", onCamera);
      map.off("resize", onCamera);
    };
  }, [map]);

  if (!map || !anchor) {
    return null;
  }

  const kind = surfaceSummary ? surfaceKindAt(surfaceSummary, content.distanceMetres) : null;

  return (
    <Marker
      // MapLibre's `Marker` has no way to change an existing instance's
      // anchor — `react-map-gl` re-applies position and offset on every
      // render but constructs the anchor once and never revisits it. Keying
      // on it forces React to tear down and rebuild the marker whenever the
      // corner it opens from changes, rather than silently keeping the first
      // one it was ever given.
      key={anchor}
      longitude={position.longitude}
      latitude={position.latitude}
      anchor={anchor}
      offset={tooltipOffset(anchor)}
      className="route-position-tooltip-marker"
    >
      <p
        ref={boxRef}
        className={`route-position-tooltip route-position-tooltip--${darkBasemap ? "dark" : "light"}`}
        aria-hidden={announce ? undefined : true}
        aria-live={announce ? "polite" : undefined}
      >
        <span>{tooltipDistance(content.distanceMetres, unitSystem)} from start</span>
        <span>
          {tooltipDistance(Math.max(endMetres - content.distanceMetres, 0), unitSystem)} to end
        </span>
        <span>{formatElevation(content.elevationMetres, unitSystem)}</span>
        <span>{content.gradientPercent.toFixed(1)}%</span>
        {kind ? <span>{SURFACE_STYLES[kind].label}</span> : null}
      </p>
    </Marker>
  );
}
