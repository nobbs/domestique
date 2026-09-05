/**
 * One scalar measure over the whole map right now — temperature, rain or
 * cloud — draped as a banded wash from the ICON-D2 grid for the current hour.
 *
 * The same bands, colours and opacities the route's own wash uses, read from
 * the measure registry, so the two never say the same reading two ways: rain
 * and cloud's lowest band stays transparent here too. An image source rather
 * than a fill per cell: a viewport is a few thousand cells, and a raster is
 * one texture however many there are.
 */

import { useMemo } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import { scalarGridReader } from "../../api/openMeteoGrid";
import { useCartography } from "../../components/map/CartographyContext";
import type { Measure } from "../../lib/measures";
import type { Rgba } from "../../lib/scalarRaster";
import { gridCorners, paintGrid } from "../../lib/scalarRaster";
import { useViewportGrid } from "./useViewportGrid";

/** Rows per grid row: enough that the Mercator resampling shows no stair-steps. */
const ROWS_PER_CELL = 2;
const OPACITY = 0.45;

function rgba(hex: string, opacity: number): Rgba {
  const value = Number.parseInt(hex.replace("#", ""), 16);

  return [(value >> 16) & 255, (value >> 8) & 255, value & 255, Math.round(opacity * 255)];
}

export interface ScalarOverlayProps {
  measure: Measure;
  /** The model's name for the measure's variable, in the units `measure.band` takes. */
  variable: string;
  on: boolean;
  /** Hours ahead of the current hour the reader has scrubbed the overlay to. */
  hoursAhead?: number;
  beforeId?: string | undefined;
}

export function ScalarOverlay({
  measure,
  variable,
  on,
  hoursAhead = 0,
  beforeId,
}: ScalarOverlayProps) {
  const { dark } = useCartography();
  const { data } = useViewportGrid(`${variable}-grid`, on, scalarGridReader(variable), hoursAhead);

  const image = useMemo(() => {
    if (!data || typeof document === "undefined") {
      return null;
    }
    const bands = measure.bands.map((_, band) =>
      rgba(measure.colour(band, dark), measure.opacity(band)),
    );
    const rows = data.ny * ROWS_PER_CELL;
    const canvas = document.createElement("canvas");
    canvas.width = data.nx;
    canvas.height = rows;
    const context = canvas.getContext("2d");
    if (!context) {
      return null;
    }
    const pixels = context.createImageData(data.nx, rows);
    paintGrid(data, rows, (value) => bands[measure.band(value)] ?? [0, 0, 0, 0], pixels.data);
    context.putImageData(pixels, 0, 0);

    return { url: canvas.toDataURL(), coordinates: gridCorners(data) };
  }, [data, dark, measure]);

  if (!on || !image) {
    return null;
  }
  const sourceId = `${measure.key}-overlay-source`;

  return (
    <Source id={sourceId} type="image" url={image.url} coordinates={image.coordinates}>
      <Layer
        id={`${measure.key}-overlay`}
        type="raster"
        source={sourceId}
        paint={{
          "raster-opacity": OPACITY,
          "raster-resampling": "linear",
          "raster-fade-duration": 0,
        }}
        {...(beforeId === undefined ? {} : { beforeId })}
      />
    </Source>
  );
}
