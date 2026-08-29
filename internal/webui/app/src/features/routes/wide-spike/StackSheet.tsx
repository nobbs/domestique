/**
 * **2 — Stack.** Every instrument at once, on one distance axis.
 *
 * Almost everything this service knows about a route is a function of one
 * variable: how far along it you are. Height is, steepness is, the ground is,
 * and — once a start time is chosen — so is the weather, because when you
 * arrive somewhere is decided by how far away it is. Four instruments plotted
 * against four separate axes is four readings a reader has to align by hand;
 * plotted against the same one, they align themselves.
 *
 * What that buys is the sentence no single instrument can say: *the rain
 * arrives on the second col, and the col is gravel*. On the selector that is
 * three views and a memory. Here it is one glance down a column.
 *
 * The cost is height. This is the tallest alternative by some way, and it
 * spends on the map exactly what the pill above it just gave back — which is
 * the trade the spike is for.
 */

import { ElevationProfile } from "../../../components/route/ElevationProfile";
import { FilmstripBand } from "../../../components/route/forecast-spike/FilmstripBand";
import { PADDING } from "../../../lib/plotAxis";
import { useElementWidth } from "../../../lib/useElementWidth";
import { groundSegments, Ribbon } from "../panel-spike/shared";
import type { SheetProps } from "./shared";
import { RideWindow, Sheet } from "./shared";

export function StackSheet({
  route,
  profile,
  surface,
  cells,
  samples,
  startAt,
  activeMetres,
  onActiveChange,
  highlight,
  unitSystem,
}: SheetProps) {
  const { ref, width } = useElementWidth<HTMLDivElement>();

  return (
    <Sheet>
      <div className="mb-2">
        <RideWindow startAt={startAt} samples={samples} />
      </div>
      <div ref={ref} className="grid gap-1.5">
        <ElevationProfile
          profile={profile}
          title={route.title}
          surface={surface}
          activeMetres={activeMetres}
          onActiveChange={onActiveChange}
          highlight={highlight}
          unitSystem={unitSystem}
        />
        {/*
         * The ribbons carry the chart's own gutters rather than running the
         * full width of the sheet. The axis is the whole argument for this
         * layout, and a ribbon starting a few pixels left of the chart above
         * it would put the gravel in the wrong place — subtly, which is worse
         * than obviously.
         */}
        {/*
         * Ground only. The chart above already paints the area under it by
         * steepness band, so a gradient ribbon here would be the same fact
         * drawn twice, one row apart — which reads as two different
         * measurements until you work out that it is not. The ground is the
         * thing the chart does not say.
         */}
        <div
          className="grid gap-1"
          style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}
        >
          <Ribbon segments={groundSegments(surface)} className="h-3" highlight={highlight} />
        </div>
        <FilmstripBand
          cells={cells}
          width={width}
          startMetres={0}
          endMetres={route.distanceMetres}
          unitSystem={unitSystem}
        />
      </div>
    </Sheet>
  );
}
