/** The route-library lines that sit over a MapWidget. */

import { useMemo } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import type { Position } from "../../api/types";
import { useCartography } from "../../components/map/CartographyContext";
import { INK as LIBRARY_INK, ROUTE_ACCENT as SELECTION_ACCENT } from "../../lib/cartography";

const LIBRARY_OPACITY = 0.68;
const CONTEXT_OPACITY = 0.14;
const HIT_WIDTH = 18;

/** The transparent layer that MapWidget asks MapLibre to pick from. */
export const LIBRARY_HIT_LAYER = "library-hit";
/** Every route as one line, always mounted, so an overlay can be ordered under it. */
export const LIBRARY_LINE_LAYER = "library-line";

export interface MapLine {
  key: string;
  coordinates: Position[];
}

export interface LibraryRoutesProps {
  lines: MapLine[];
  /** The route that makes the rest of the library contextual. */
  pickedKey: string | null;
  /**
   * Whether the picked route has its own overlay, which puts the library away
   * entirely: the routes that share its roads were being hit instead of it.
   */
  overlaid?: boolean;
  hoveredKey?: string | null;
  /** Mount the transparent target under this id when the parent handles picks. */
  hitLayerId?: string;
}

function collectionOf(lines: MapLine[]) {
  return {
    type: "FeatureCollection" as const,
    features: lines
      .filter((line) => line.coordinates.length > 1)
      .map((line) => ({
        type: "Feature" as const,
        geometry: { type: "LineString" as const, coordinates: line.coordinates },
        properties: { key: line.key },
      })),
  };
}

export function LibraryRoutes({
  lines,
  pickedKey,
  overlaid = false,
  hoveredKey = null,
  hitLayerId,
}: LibraryRoutesProps) {
  const { dark } = useCartography();
  const library = useMemo(() => collectionOf(lines), [lines]);
  const theme = dark ? "dark" : "light";

  return (
    <Source id="library-lines" type="geojson" data={library}>
      {/*
       * Hidden rather than unmounted while the overlay is up: the source stays
       * uploaded, so leaving a route does not re-upload the whole library.
       */}
      <Layer
        id={LIBRARY_LINE_LAYER}
        type="line"
        layout={{
          "line-cap": "round",
          "line-join": "round",
          visibility: overlaid ? "none" : "visible",
        }}
        paint={{
          "line-color": LIBRARY_INK[theme],
          "line-width": 2,
          "line-opacity": pickedKey === null ? LIBRARY_OPACITY : CONTEXT_OPACITY,
        }}
      />
      {!overlaid && pickedKey !== null ? (
        <Layer
          id="library-selected-line"
          type="line"
          filter={["==", ["get", "key"], pickedKey]}
          layout={{ "line-cap": "round", "line-join": "round" }}
          paint={{ "line-color": SELECTION_ACCENT[theme], "line-width": 3, "line-opacity": 1 }}
        />
      ) : null}
      {!overlaid && hoveredKey !== null && hoveredKey !== pickedKey ? (
        <Layer
          id="library-hover-line"
          type="line"
          filter={["==", ["get", "key"], hoveredKey]}
          layout={{ "line-cap": "round", "line-join": "round" }}
          paint={{ "line-color": SELECTION_ACCENT[theme], "line-width": 3, "line-opacity": 0.75 }}
        />
      ) : null}
      {hitLayerId && !overlaid ? (
        <Layer
          id={hitLayerId}
          type="line"
          layout={{ "line-cap": "round", "line-join": "round" }}
          paint={{ "line-color": LIBRARY_INK[theme], "line-width": HIT_WIDTH, "line-opacity": 0 }}
        />
      ) : null}
    </Source>
  );
}
