/**
 * **4 — Timeline.** The same ride, against the clock instead of the tape.
 *
 * Every other alternative plots distance because that is how a route is
 * stored. But the questions a rider actually asks the day before are asked in
 * hours: *where am I when that front arrives at two*, *is the last col in the
 * dark*, *do I clear the gravel before the rain*. Distance answers those only
 * after arithmetic the panel is already doing internally — the forecast exists
 * at all because the service converts one into the other.
 *
 * So the axis is the time of day, the profile is redrawn against it, and the
 * cols become the hours they will take. The shape changes in a way that is the
 * point rather than a distortion: a climb is wide here and a descent is narrow,
 * because that is what they are to ride. Twelve flat kilometres before lunch
 * take less of the day than four kilometres of col.
 *
 * The cost is that it stops being a map of the route. Distance is what the
 * chart, the map and the highlight all address ground by, so this view cannot
 * be scrubbed against them without converting back — and it is wrong the
 * moment the start time changes, where every other view is merely re-shaded.
 */

import type { ForecastSample } from "../../../lib/forecastSamples";
import { formatDistance } from "../../../lib/format";
import { temperatureColour, weatherIcon } from "../../../lib/weather";
import type { SheetProps } from "./shared";
import { clockAt, RideWindow, Sheet } from "./shared";

const PLOT_HEIGHT = 96;
const STRIP_HEIGHT = 34;
const AXIS_HEIGHT = 16;
const HEIGHT = PLOT_HEIGHT + STRIP_HEIGHT + AXIS_HEIGHT;
const LEFT = 34;
const RIGHT = 12;

/** Room over the highest point so the summit is not drawn on the top edge. */
const HEADROOM = 40;

/**
 * When the ride reaches a distance, from the arrival times it was sampled at.
 *
 * The samples are half an hour of moving time apart, so between two of them a
 * straight line is very nearly right — and being exactly right would mean
 * re-running the speed model here, against a profile it did not produce.
 */
function timeFor(samples: ForecastSample[], metres: number): number {
  const after = samples.findIndex((sample) => sample.distanceMetres >= metres);
  if (after <= 0) {
    return samples[0]?.arrivalAt.getTime() ?? 0;
  }
  const previous = samples[after - 1];
  const next = samples[after];
  if (!previous || !next) {
    return samples[samples.length - 1]?.arrivalAt.getTime() ?? 0;
  }
  const span = next.distanceMetres - previous.distanceMetres;
  const fraction = span > 0 ? (metres - previous.distanceMetres) / span : 0;

  return (
    previous.arrivalAt.getTime() +
    (next.arrivalAt.getTime() - previous.arrivalAt.getTime()) * fraction
  );
}

/** Every whole hour the ride is out for, as the axis. */
function hoursBetween(from: number, to: number): number[] {
  const first = new Date(from);
  first.setMinutes(0, 0, 0);
  const hours: number[] = [];
  for (let at = first.getTime(); at <= to; at += 3_600_000) {
    if (at >= from) {
      hours.push(at);
    }
  }

  return hours;
}

export function TimelineSheet({
  route,
  profile,
  climbs,
  cells,
  samples,
  startAt,
  unitSystem,
}: SheetProps) {
  const first = startAt.getTime();
  const last = samples[samples.length - 1]?.arrivalAt.getTime() ?? first + 3_600_000;
  const span = Math.max(last - first, 1);
  const x = (at: number) => LEFT + ((at - first) / span) * (1000 - LEFT - RIGHT);

  const elevations = profile?.samples.map((sample) => sample.elevationMetres) ?? [];
  const floor = elevations.length > 0 ? Math.min(...elevations) : 0;
  const ceiling = (elevations.length > 0 ? Math.max(...elevations) : 1) + HEADROOM;
  const y = (metres: number) =>
    PLOT_HEIGHT - ((metres - floor) / Math.max(ceiling - floor, 1)) * PLOT_HEIGHT;

  const line = (profile?.samples ?? [])
    .map(
      (sample, index) =>
        `${index === 0 ? "M" : "L"}${x(timeFor(samples, sample.distanceMetres)).toFixed(1)} ${y(sample.elevationMetres).toFixed(1)}`,
    )
    .join(" ");

  return (
    <Sheet>
      <div className="mb-2">
        <RideWindow startAt={startAt} samples={samples} />
      </div>
      <svg
        width="100%"
        height={HEIGHT}
        viewBox={`0 0 1000 ${HEIGHT}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={`The ride against the clock, ${clockAt(startAt)} to ${clockAt(new Date(last))}`}
        className="block"
      >
        <title>{`The ride against the clock, ${clockAt(startAt)} to ${clockAt(new Date(last))}`}</title>
        {/*
         * The cols as the hours they take. Wide bands rather than marks: on
         * this axis a climb is not a place, it is a chunk of the afternoon.
         */}
        {climbs.map((climb) => {
          const from = x(timeFor(samples, climb.startMetres));
          const to = x(timeFor(samples, climb.endMetres));

          return (
            <rect
              key={climb.startMetres}
              x={from}
              y={0}
              width={Math.max(to - from, 1)}
              height={PLOT_HEIGHT}
              className="fill-[var(--ink-2)]"
              style={{ fillOpacity: 0.08 }}
            />
          );
        })}
        <path
          d={`${line} L${x(last).toFixed(1)} ${PLOT_HEIGHT} L${x(first).toFixed(1)} ${PLOT_HEIGHT} Z`}
          className="fill-[var(--accent)]"
          style={{ fillOpacity: 0.14 }}
        />
        <path d={line} className="fill-none stroke-[var(--accent)]" strokeWidth={1.6} />
        {hoursBetween(first, last).map((at) => (
          <g key={at}>
            <line
              x1={x(at)}
              x2={x(at)}
              y1={0}
              y2={PLOT_HEIGHT}
              className="stroke-[var(--rule)]"
              strokeWidth={1}
            />
            <text
              x={x(at)}
              y={HEIGHT - 4}
              textAnchor="middle"
              className="fill-[var(--ink-2)] text-[10px] tabular-nums"
            >
              {clockAt(new Date(at))}
            </text>
          </g>
        ))}
        {/*
         * The forecast on the same axis, which on this one it is native to:
         * every reading is a moment, and the tiles fall where the moments are
         * without any conversion at all.
         */}
        {cells.map((cell) => {
          const Glyph = weatherIcon(cell.point.weatherCode);

          return (
            <g
              key={cell.sample.arrivalAt.getTime()}
              transform={`translate(${x(cell.sample.arrivalAt.getTime())} ${PLOT_HEIGHT})`}
            >
              <text
                y={14}
                textAnchor="middle"
                className="text-[10px] font-semibold tabular-nums"
                fill={temperatureColour(cell.point.temperatureCelsius)}
              >
                {`${Math.round(cell.point.temperatureCelsius)}°`}
              </text>
              <foreignObject x={-8} y={17} width={16} height={16}>
                <Glyph size={16} stroke={1.6} aria-hidden="true" />
              </foreignObject>
            </g>
          );
        })}
      </svg>
      <p className="mt-1 text-[11px] text-[var(--ink-2)]">
        Shaded hours are the climbs · {formatDistance(route.distanceMetres, unitSystem)} in all
      </p>
    </Sheet>
  );
}
