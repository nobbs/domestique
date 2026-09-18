/** The planner's line between routes: straight where a leg awaits routing, and a morph when a route arrives. */

import { useEffect, useRef, useState } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import type { PlanWaypoint, Position } from "../../api/types";
import { useCartography } from "../../components/map/CartographyContext";
import { ROUTE_ACCENT, PANEL as ROUTE_CASING } from "../../lib/cartography";
import { coordinateRange } from "../../lib/profile";

/** One leg as drawn; a provisional one is a straight line to a waypoint the router has not seen yet. */
export interface DrawnLeg {
  coordinates: Position[];
  provisional: boolean;
}

type IdentifiedWaypoint = PlanWaypoint & { id: number };

const MORPH_MS = 450;
const MAX_MORPH_POINTS = 256;

/** A routed line cut into one run per leg, at the distance the line reaches each waypoint. */
export function routedLegs(line: Position[], progress: number[] | undefined): Position[][] | null {
  if (!progress || progress.length < 2 || line.length < 2) {
    return null;
  }

  return progress.slice(1).map((end, index) => {
    const range = coordinateRange(line, progress[index] ?? 0, end);
    return range ? line.slice(range.startIndex, range.endIndex + 1) : [];
  });
}

function samePlace(a: IdentifiedWaypoint | undefined, b: IdentifiedWaypoint): boolean {
  return a?.id === b.id && a.longitude === b.longitude && a.latitude === b.latitude;
}

/**
 * The legs between `waypoints` as they stand: a leg the last route covered
 * keeps its routed shape, and any other is a straight provisional line.
 */
export function provisionalLegs(
  waypoints: IdentifiedWaypoint[],
  routedFor: IdentifiedWaypoint[] | null,
  routed: Position[][] | null,
  line: Position[],
): DrawnLeg[] {
  // A preview without per-waypoint distances cannot be cut; it stands whole while nothing moved.
  if (
    !routed &&
    line.length > 1 &&
    routedFor?.length === waypoints.length &&
    waypoints.every(
      (waypoint, index) =>
        samePlace(routedFor[index], waypoint) &&
        Boolean(routedFor[index]?.straight) === Boolean(waypoint.straight),
    )
  ) {
    return [{ coordinates: line, provisional: false }];
  }
  return waypoints.slice(1).flatMap((to, index): DrawnLeg[] => {
    const from = waypoints[index];
    if (!from) {
      return [];
    }
    const at = routedFor?.findIndex((waypoint) => waypoint.id === from.id) ?? -1;
    const leg = routed?.[at];
    const next = routedFor?.[at + 1];
    if (
      at >= 0 &&
      leg &&
      leg.length > 1 &&
      samePlace(routedFor?.[at], from) &&
      next &&
      samePlace(next, to) &&
      Boolean(next.straight) === Boolean(to.straight)
    ) {
      return [{ coordinates: leg, provisional: false }];
    }

    return [
      {
        coordinates: [
          [from.longitude, from.latitude],
          [to.longitude, to.latitude],
        ],
        provisional: true,
      },
    ];
  });
}

/** `count` points spread evenly along a line's length, in plain degrees; close enough for a leg. */
export function resample(coordinates: Position[], count: number): Position[] {
  const lengths = [0];
  for (let index = 1; index < coordinates.length; index++) {
    const [ax = 0, ay = 0] = coordinates[index - 1] ?? [];
    const [bx = 0, by = 0] = coordinates[index] ?? [];
    lengths.push((lengths[index - 1] ?? 0) + Math.hypot(bx - ax, by - ay));
  }
  const total = lengths.at(-1) ?? 0;
  let segment = 1;

  return Array.from({ length: count }, (_, index) => {
    const along = count === 1 ? 0 : (index / (count - 1)) * total;
    while (segment < coordinates.length - 1 && (lengths[segment] ?? 0) < along) {
      segment++;
    }
    const [ax = 0, ay = 0] = coordinates[segment - 1] ?? coordinates[0] ?? [];
    const [bx = ax, by = ay] = coordinates[segment] ?? [];
    const start = lengths[segment - 1] ?? 0;
    const span = (lengths[segment] ?? start) - start;
    const fraction = span === 0 ? 0 : (along - start) / span;

    return [ax + (bx - ax) * fraction, ay + (by - ay) * fraction] as Position;
  });
}

/** The legs `fraction` of the way from one shape to the other, leg by leg. */
export function interpolateLegs(from: DrawnLeg[], to: DrawnLeg[], fraction: number): DrawnLeg[] {
  return to.map((target, index) => {
    const origin = from[index]?.coordinates ?? target.coordinates;
    const count = Math.min(MAX_MORPH_POINTS, Math.max(2, origin.length, target.coordinates.length));
    const a = resample(origin, count);
    const b = resample(target.coordinates, count);

    return {
      provisional: false,
      coordinates: a.map(([ax = 0, ay = 0], point) => {
        const [bx = ax, by = ay] = b[point] ?? [];
        return [ax + (bx - ax) * fraction, ay + (by - ay) * fraction] as Position;
      }),
    };
  });
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/**
 * Draws `legs`; when `morph` is set, it eases from whatever it drew last into
 * them, then calls `onMorphed`. Stays mounted while hidden so the next morph
 * starts from the line the rider last saw.
 */
export function RouteTransition({
  legs,
  morph,
  visible,
  onMorphed,
}: {
  legs: DrawnLeg[];
  morph: boolean;
  visible: boolean;
  onMorphed: () => void;
}) {
  const { dark } = useCartography();
  const [drawn, setDrawn] = useState(legs);
  const last = useRef(legs);
  const done = useRef(onMorphed);
  done.current = onMorphed;

  useEffect(() => {
    const draw = (next: DrawnLeg[]) => {
      last.current = next;
      setDrawn(next);
    };
    const from = last.current;
    if (!morph || from.length !== legs.length || prefersReducedMotion()) {
      draw(legs);
      if (morph) {
        done.current();
      }
      return;
    }
    const start = performance.now();
    let frame = requestAnimationFrame(function step(now) {
      const t = Math.min(1, (now - start) / MORPH_MS);
      draw(t < 1 ? interpolateLegs(from, legs, 1 - (1 - t) ** 3) : legs);
      if (t < 1) {
        frame = requestAnimationFrame(step);
      } else {
        done.current();
      }
    });

    return () => cancelAnimationFrame(frame);
  }, [legs, morph]);

  const data = {
    type: "FeatureCollection" as const,
    features: drawn.map((leg) => ({
      type: "Feature" as const,
      properties: { provisional: leg.provisional },
      geometry: { type: "LineString" as const, coordinates: leg.coordinates },
    })),
  };
  const opacity = visible ? 1 : 0;

  return (
    <Source id="plan-transition" type="geojson" data={data}>
      <Layer
        id="plan-transition-casing"
        type="line"
        filter={["!", ["get", "provisional"]]}
        layout={{ "line-cap": "round", "line-join": "round" }}
        paint={{
          "line-color": ROUTE_CASING[dark ? "dark" : "light"],
          "line-width": 7,
          "line-opacity": opacity,
        }}
      />
      <Layer
        id="plan-transition-line"
        type="line"
        filter={["!", ["get", "provisional"]]}
        layout={{ "line-cap": "round", "line-join": "round" }}
        paint={{
          "line-color": ROUTE_ACCENT[dark ? "dark" : "light"],
          "line-width": 4,
          "line-opacity": opacity,
        }}
      />
      <Layer
        id="plan-transition-pending"
        type="line"
        filter={["get", "provisional"]}
        layout={{ "line-cap": "round" }}
        paint={{
          "line-color": ROUTE_ACCENT[dark ? "dark" : "light"],
          "line-width": 3,
          "line-dasharray": [1, 2],
          "line-opacity": opacity,
        }}
      />
    </Source>
  );
}
