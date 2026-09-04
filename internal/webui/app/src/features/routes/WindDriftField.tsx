/**
 * Which way the air is actually moving, as streaks drifting through the
 * corridor.
 *
 * The last of the three things the wind is. `ConditionsWash` paints how hard it
 * blows and `WindRelationTint` what that does to the rider; neither says where
 * the air is going over the ground, and a colour for a bearing would be a
 * legend to look up. Motion is the one channel that needs no key at all: a
 * streak that drifts south is read as a wind out of the north without a word.
 *
 * Quiet on purpose. It carries direction and nothing else — the corridor
 * beneath it already has the speed and the route above it already has the
 * relation — so it is drawn faint and thin enough to be seen out of the corner
 * of the eye while the reader looks at something else.
 *
 * Under the route, in the same slot the wash uses, because particles over the
 * line would obscure the thing the reader is following. That is what makes this
 * a MapLibre custom layer rather than a canvas over the map: only a layer in
 * the map's own stack can take a `beforeId` (`windStreakLayer.ts`).
 *
 * Nothing moves for a reader who asked for no movement. There is no slowed-down
 * field for that case: `staticFlow` puts a dozen arrows in the corridor instead,
 * pointing the same way, and no frame is ever requested.
 *
 * Runs its own `weatherQuery`, the way the wash and the tint do: React Query
 * keys on the samples, so all of them share one cache entry.
 */

import { IconArrowNarrowUp } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Layer, Marker, useMap } from "react-map-gl/maplibre";
import { weatherQuery } from "../../api/queries";
import type { Position } from "../../api/types";
import { useCartography } from "../../components/map/CartographyContext";
import { PANEL } from "../../lib/cartography";
import { arrivalResolution } from "../../lib/forecastResolution";
import type { ForecastSample } from "../../lib/forecastSamples";
import type { MeasureKey } from "../../lib/measures";
import { usePrefersReducedMotion } from "../../lib/mediaQuery";
import { cumulativeMetres } from "../../lib/profile";
import type { FieldGeometry } from "../../lib/windField";
import {
  advanceField,
  FLOATS_PER_VERTEX,
  fieldSize,
  seedField,
  staticFlow,
  VERTICES_PER_STREAK,
  writeStreaks,
} from "../../lib/windField";
import type { StreakFrame } from "./windStreakLayer";
import { shaderColour, windStreakLayer } from "./windStreakLayer";

/** The field's own layer, so a caller ordering the stack has a name to use. */
export const WIND_FIELD_LAYER_ID = "route-wind-field";

/**
 * How strongly the whole field is drawn, over the alpha a streak already
 * carries for its life and its place in the corridor.
 *
 * The direction channel, and only that. At full strength a few hundred streaks
 * read as a texture over the wash and start competing with the band colour they
 * are drawn on top of, which is the reading this layer is least able to spare.
 */
export const FIELD_STRENGTH = 0.7;

/**
 * The longest step the field is ever advanced by, in seconds.
 *
 * `requestAnimationFrame` stops while a tab is in the background and the first
 * frame back reports the whole absence. Without this the field would jump a
 * quarter of an hour of drift in one step and every particle would be somewhere
 * else, which reads as the map having been redrawn rather than as wind.
 */
const MAX_FRAME_SECONDS = 0.1;

export interface WindDriftFieldProps {
  /** The route's stored geometry, which the corridor is laid along. */
  coordinates: Position[];
  /**
   * The forecast requests for this ride. Empty until a start time is picked,
   * and empty for a stage nothing has predicted a moving time for.
   */
  samples: ForecastSample[];
  /** The measure the reader asked for. Only `"wind"` draws anything here. */
  measure: MeasureKey | null;
  /** The layer the field is drawn beneath, so it stays under the route itself. */
  beforeId?: string | undefined;
}

export function WindDriftField({ coordinates, samples, measure, beforeId }: WindDriftFieldProps) {
  const { dark: darkBasemap } = useCartography();
  const { current: map } = useMap();
  const reducedMotion = usePrefersReducedMotion();
  const wanted = measure === "wind";
  // Only for the wind, and only while it is asked for: the wash and the strip
  // make this same request under the same key, so this shares their cache
  // rather than fetching a third time.
  const forecast = useQuery({
    ...weatherQuery(samples),
    enabled: wanted && samples.length > 0,
  });
  const points = forecast.data?.points;

  const distances = useMemo(() => cumulativeMetres(coordinates), [coordinates]);
  const totalMetres = distances[distances.length - 1] ?? 0;
  // Not memoized: it reads Date.now(), so caching it against [samples] would
  // let this drift from ConditionsWash's and ForecastStrip's own resolution
  // once enough real time passes between renders to cross a lead-time band.
  const metresPerCell = arrivalResolution(samples[0]?.arrivalAt).metresPerCell;

  // Everything the field drifts through, or null when there is nothing to
  // drift: another measure, a ride with no forecast, a forecast still in
  // flight. `weatherQuery` has already refused a response whose points do not
  // match the samples 1:1.
  const geometry = useMemo<FieldGeometry | null>(() => {
    if (!wanted || !points || totalMetres <= 0) {
      return null;
    }
    const readings = samples.flatMap((sample, index) => {
      const point = points[index];

      return point
        ? [
            {
              distanceMetres: sample.distanceMetres,
              speedKmh: point.windSpeedKmh,
              directionDegrees: point.windDirectionDegrees,
            },
          ]
        : [];
    });

    return readings.length > 0
      ? { coordinates, distances, samples: readings, metresPerCell, totalMetres }
      : null;
  }, [coordinates, distances, metresPerCell, points, samples, totalMetres, wanted]);

  // Ink that contrasts with the ground it is on, rather than the casing colour
  // the chevrons take: a streak has no outline to carry it against a basemap of
  // its own brightness, and there is no room to give a few hundred of them one.
  const inkHex = PANEL[darkBasemap ? "light" : "dark"];
  const colour = useMemo(() => shaderColour(inkHex), [inkHex]);
  // The frame the layer reads at the moment it draws. Mutated in place rather
  // than replaced: a new object per frame is a few thousand allocations a
  // minute handed to the collector while the map is already drawing.
  const frame = useRef<StreakFrame>({
    vertices: new Float32Array(0),
    vertexCount: 0,
    colour,
    strength: FIELD_STRENGTH,
  });
  useEffect(() => {
    frame.current.colour = colour;
  }, [colour]);
  // Made once and never again: `react-map-gl` adds the layer it is first handed
  // and ignores every later one with the same id, so a layer rebuilt on a
  // render would silently stop being the one the map is drawing.
  const [streaks] = useState(() => windStreakLayer(WIND_FIELD_LAYER_ID, () => frame.current));

  useEffect(() => {
    if (!geometry || !map || reducedMotion) {
      return;
    }
    const particles = seedField(geometry, fieldSize(geometry.totalMetres));
    const vertices = new Float32Array(particles.length * VERTICES_PER_STREAK * FLOATS_PER_VERTEX);
    frame.current.vertices = vertices;
    frame.current.vertexCount = 0;

    let handle: number | null = null;
    let previous: number | null = null;
    const step = (now: number) => {
      handle = requestAnimationFrame(step);
      const seconds = previous === null ? 0 : Math.min((now - previous) / 1000, MAX_FRAME_SECONDS);
      previous = now;
      advanceField(particles, geometry, seconds);
      frame.current.vertexCount = writeStreaks(particles, geometry, vertices);
      // The layer draws when the map draws, so the map has to be asked. This is
      // the only thing a frame of this animation does to MapLibre.
      map.triggerRepaint();
    };
    const start = () => {
      if (handle === null) {
        previous = null;
        handle = requestAnimationFrame(step);
      }
    };
    const stop = () => {
      if (handle !== null) {
        cancelAnimationFrame(handle);
        handle = null;
      }
    };
    // A hidden tab still runs its timers even where it stops serving frames,
    // and the field has nothing to say to a reader who is not looking at it.
    const visibility = () => {
      if (document.hidden) {
        stop();
      } else {
        start();
      }
    };
    document.addEventListener("visibilitychange", visibility);
    if (!document.hidden) {
      start();
    }

    return () => {
      document.removeEventListener("visibilitychange", visibility);
      stop();
      // The buffer belongs to the route that has just gone: drawn again against
      // another one it would be a field of streaks over the wrong ground.
      frame.current.vertexCount = 0;
    };
  }, [geometry, map, reducedMotion]);

  const arrows = useMemo(
    () => (geometry && reducedMotion ? staticFlow(geometry) : []),
    [geometry, reducedMotion],
  );

  if (!geometry) {
    return null;
  }

  if (reducedMotion) {
    return (
      <>
        {arrows.map((arrow) => (
          <Marker
            key={arrow.distanceMetres}
            longitude={arrow.position[0]}
            latitude={arrow.position[1]}
            anchor="center"
            className="route-wind-arrow-marker"
          >
            {/*
             * The wash and the tint already report this forecast in words, and
             * an arrow says nothing they do not — so it is a picture and only a
             * picture, and a third table would be the same forecast a third
             * time for a reader who cannot see any of them.
             */}
            <IconArrowNarrowUp
              className={`route-wind-arrow route-wind-arrow--${darkBasemap ? "dark" : "light"}`}
              color={inkHex}
              size={24}
              stroke={2.5}
              style={{ transform: `rotate(${arrow.bearingDegrees}deg)` }}
              aria-hidden="true"
            />
          </Marker>
        ))}
      </>
    );
  }

  return (
    <Layer
      {...streaks}
      // Spread only when there is one. Under `exactOptionalPropertyTypes` an
      // optional prop and one that may be undefined are different types.
      {...(beforeId === undefined ? {} : { beforeId })}
    />
  );
}
