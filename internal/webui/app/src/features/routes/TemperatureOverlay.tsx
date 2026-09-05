/**
 * The air temperature over the whole map right now, draped as a banded wash
 * from the ICON-D2 grid for the current hour.
 *
 * The same five bands and colours the route's own temperature wash uses, so
 * the two never say the same reading two ways. An image source rather than a
 * fill per cell: a viewport is a few thousand cells, and a raster is one
 * texture however many there are.
 */

import { useMemo } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import { fetchTemperatureGrid } from "../../api/openMeteoGrid";
import { useCartography } from "../../components/map/CartographyContext";
import { temperatureHexColour } from "../../lib/measures";
import type { Rgba } from "../../lib/scalarRaster";
import { gridCorners, paintGrid } from "../../lib/scalarRaster";
import { temperatureBand } from "../../lib/weather";
import { useViewportGrid } from "./useViewportGrid";

export const TEMPERATURE_OVERLAY_LAYER_ID = "temperature-overlay";
const SOURCE_ID = "temperature-overlay-source";
/** Rows per grid row: enough that the Mercator resampling shows no stair-steps. */
const ROWS_PER_CELL = 2;
const OPACITY = 0.45;

function rgba(hex: string): Rgba {
  const value = Number.parseInt(hex.replace("#", ""), 16);

  return [(value >> 16) & 255, (value >> 8) & 255, value & 255, 255];
}

export interface TemperatureOverlayProps {
  on: boolean;
  beforeId?: string | undefined;
}

export function TemperatureOverlay({ on, beforeId }: TemperatureOverlayProps) {
  const { dark } = useCartography();
  const { data } = useViewportGrid("temperature-grid", on, fetchTemperatureGrid);

  const image = useMemo(() => {
    if (!data || typeof document === "undefined") {
      return null;
    }
    const bands = [0, 1, 2, 3, 4].map((band) => rgba(temperatureHexColour(band, dark)));
    const rows = data.ny * ROWS_PER_CELL;
    const canvas = document.createElement("canvas");
    canvas.width = data.nx;
    canvas.height = rows;
    const context = canvas.getContext("2d");
    if (!context) {
      return null;
    }
    const pixels = context.createImageData(data.nx, rows);
    paintGrid(
      data,
      rows,
      (celsius) => bands[temperatureBand(celsius)] ?? [0, 0, 0, 0],
      pixels.data,
    );
    context.putImageData(pixels, 0, 0);

    return { url: canvas.toDataURL(), coordinates: gridCorners(data) };
  }, [data, dark]);

  if (!on || !image) {
    return null;
  }

  return (
    <Source id={SOURCE_ID} type="image" url={image.url} coordinates={image.coordinates}>
      <Layer
        id={TEMPERATURE_OVERLAY_LAYER_ID}
        type="raster"
        source={SOURCE_ID}
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
