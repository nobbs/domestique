/** The route-library lines that sit over a MapWidget. */

import { useMemo } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import type { Position } from "../../api/types";

const LIBRARY_INK = { light: "#1c2126", dark: "#eef0f3" } as const;
const SELECTION_ACCENT = { light: "#236fc7", dark: "#70adfb" } as const;

const LIBRARY_OPACITY = 0.68;
const CONTEXT_OPACITY = 0.14;
const HIT_WIDTH = 18;

/** The transparent layer that MapWidget asks MapLibre to pick from. */
export const LIBRARY_HIT_LAYER = "library-hit";

export interface MapLine {
  key: string;
  coordinates: Position[];
}

export interface LibraryRoutesProps {
  lines: MapLine[];
  darkBasemap?: boolean;
  /** The route that makes the rest of the library contextual. */
  selectedKey: string | null;
  /** The route painted in the accent while its full overlay is absent. */
  accentKey?: string | null;
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
  darkBasemap = false,
  selectedKey,
  accentKey = selectedKey,
  hoveredKey = null,
  hitLayerId,
}: LibraryRoutesProps) {
  const library = useMemo(() => collectionOf(lines), [lines]);
  const theme = darkBasemap ? "dark" : "light";

  return (
    <Source id="library-lines" type="geojson" data={library}>
      <Layer
        id="library-line"
        type="line"
        layout={{ "line-cap": "round", "line-join": "round" }}
        paint={{
          "line-color": LIBRARY_INK[theme],
          "line-width": 2,
          "line-opacity": selectedKey === null ? LIBRARY_OPACITY : CONTEXT_OPACITY,
        }}
      />
      {accentKey !== null ? (
        <Layer
          id="library-selected-line"
          type="line"
          filter={["==", ["get", "key"], accentKey]}
          layout={{ "line-cap": "round", "line-join": "round" }}
          paint={{ "line-color": SELECTION_ACCENT[theme], "line-width": 3, "line-opacity": 1 }}
        />
      ) : null}
      {hoveredKey !== null && hoveredKey !== accentKey ? (
        <Layer
          id="library-hover-line"
          type="line"
          filter={["==", ["get", "key"], hoveredKey]}
          layout={{ "line-cap": "round", "line-join": "round" }}
          paint={{ "line-color": SELECTION_ACCENT[theme], "line-width": 3, "line-opacity": 0.75 }}
        />
      ) : null}
      {hitLayerId ? (
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
