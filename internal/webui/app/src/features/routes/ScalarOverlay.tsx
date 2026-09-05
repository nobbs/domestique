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

import { useEffect, useState } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import { scalarGridReader } from "../../api/openMeteoGrid";
import { useCartography } from "../../components/map/CartographyContext";
import type { Measure } from "../../lib/measures";
import type { Corners, Rgba } from "../../lib/scalarRaster";
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
  const [image, setImage] = useState<{ url: string; coordinates: Corners } | null>(null);

  useEffect(() => {
    if (!data || typeof document === "undefined") {
      setImage(null);

      return;
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
      // The previous grid's URL was just revoked by the effect this one
      // replaced; leaving `image` as it was would hand the map a source
      // already pointing at nothing.
      setImage(null);

      return;
    }
    const pixels = context.createImageData(data.nx, rows);
    paintGrid(data, rows, (value) => bands[measure.band(value)] ?? [0, 0, 0, 0], pixels.data);
    context.putImageData(pixels, 0, 0);

    // A blob and an object URL, not `toDataURL`: that encodes the whole
    // raster into a base64 string synchronously on the main thread, on every
    // pan and every hour scrubbed. `toBlob` hands back binary off that
    // thread, and the URL it is given is revoked once this effect is done
    // with it — by the next grid, or by the overlay going away.
    let cancelled = false;
    let objectUrl: string | null = null;
    canvas.toBlob((blob) => {
      if (cancelled) {
        return;
      }
      if (!blob) {
        // Same reasoning as the missing context above: the URL this effect
        // would have replaced is already gone.
        setImage(null);

        return;
      }
      objectUrl = URL.createObjectURL(blob);
      setImage({ url: objectUrl, coordinates: gridCorners(data) });
    });

    return () => {
      cancelled = true;
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
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
