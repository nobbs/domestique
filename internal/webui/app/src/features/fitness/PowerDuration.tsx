/**
 * The power-duration curve over the window, against the same curve over the
 * window of equal length before it where the service had one.
 *
 * Drawn on a logarithmic time axis, because the durations span five seconds to
 * an hour. The two curves sit a few percent apart, which no watt axis shows, so
 * the change at each duration gets its own strip and its own column.
 */

import type { PowerCurvePoint } from "../../api/types";
import { ChartLegend, niceTicks } from "../../components/chart/TimeFrame";
import { signed } from "./form";
import { FITNESS_COLOUR } from "./SeasonChart";

const WIDTH = 520;
const HEIGHT = 180;
const STRIP = 56;
const PAD = { left: 40, right: 16, top: 10, bottom: 22 };

/** How a duration is said on the axis and in the table: 5s, 1m, 1h. */
export function formatCurveDuration(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`;
  }

  return seconds < 3600 ? `${Math.round(seconds / 60)}m` : `${Math.round(seconds / 3600)}h`;
}

interface Props {
  current: readonly PowerCurvePoint[];
  previous: readonly PowerCurvePoint[] | undefined;
  /** What the earlier window is called, "the 6 months before". */
  previousName: string;
}

export function PowerDuration({ current, previous, previousName }: Props) {
  const first = current[0];
  const last = current[current.length - 1];
  if (!first || !last) {
    return null;
  }
  const before = new Map((previous ?? []).map((point) => [point.seconds, point.watts]));
  const span = Math.log(last.seconds) - Math.log(first.seconds) || 1;
  const x = (seconds: number) =>
    PAD.left +
    ((Math.log(seconds) - Math.log(first.seconds)) / span) * (WIDTH - PAD.left - PAD.right);
  const top =
    Math.ceil(Math.max(...current.map((one) => one.watts), ...before.values()) / 200) * 200 || 200;
  const y = (watts: number) => PAD.top + (1 - watts / top) * (HEIGHT - PAD.top - PAD.bottom);
  const line = (curve: readonly PowerCurvePoint[]) =>
    curve.map((one) => `${x(one.seconds)},${y(one.watts)}`).join(" ");
  const changes = current.flatMap((one) => {
    const earlier = before.get(one.seconds);
    return earlier
      ? [{ seconds: one.seconds, percent: ((one.watts - earlier) / earlier) * 100 }]
      : [];
  });
  const reach = Math.max(5, ...changes.map((one) => Math.abs(one.percent)));

  return (
    <>
      <ChartLegend
        items={[
          { label: "This range", colour: FITNESS_COLOUR },
          ...(previous ? [{ label: previousName, colour: "var(--ink-2)", dashed: true }] : []),
        ]}
      />
      <div className="grid items-start gap-4 md:grid-cols-[1fr_12rem]">
        <div className="flex flex-col gap-1">
          <svg
            viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
            className="w-full"
            role="img"
            aria-label={`Best mean power over ${current.map((one) => formatCurveDuration(one.seconds)).join(", ")}`}
          >
            {niceTicks(0, top, 4).map((tick) => (
              <g key={tick}>
                <line
                  x1={PAD.left}
                  x2={WIDTH - PAD.right}
                  y1={y(tick)}
                  y2={y(tick)}
                  stroke="var(--rule)"
                  strokeWidth={tick === 0 ? 1 : 0.5}
                  strokeDasharray={tick === 0 ? undefined : "2 3"}
                />
                <text
                  x={PAD.left - 6}
                  y={y(tick)}
                  dy="0.32em"
                  textAnchor="end"
                  className="fill-[var(--ink-2)] text-[10px] tabular-nums"
                >
                  {tick}
                </text>
              </g>
            ))}
            {current.map((one) => (
              <text
                key={one.seconds}
                x={x(one.seconds)}
                y={HEIGHT - 6}
                textAnchor="middle"
                className="fill-[var(--ink-2)] text-[10px]"
              >
                {formatCurveDuration(one.seconds)}
              </text>
            ))}
            {previous && previous.length > 0 ? (
              <polyline
                points={line(previous)}
                fill="none"
                stroke="var(--ink-2)"
                strokeWidth={1.5}
                strokeDasharray="4 3"
              />
            ) : null}
            <polyline points={line(current)} fill="none" stroke={FITNESS_COLOUR} strokeWidth={2} />
            {current.map((one) => (
              <circle
                key={one.seconds}
                cx={x(one.seconds)}
                cy={y(one.watts)}
                r={3.5}
                fill={FITNESS_COLOUR}
                stroke="var(--panel)"
                strokeWidth={2}
              />
            ))}
          </svg>
          {changes.length > 0 ? (
            <svg
              viewBox={`0 0 ${WIDTH} ${STRIP}`}
              className="w-full"
              role="img"
              aria-label={`Change on ${previousName}, per duration`}
            >
              <line
                x1={PAD.left}
                x2={WIDTH - PAD.right}
                y1={STRIP / 2}
                y2={STRIP / 2}
                stroke="var(--rule)"
              />
              <text
                x={PAD.left - 6}
                y={STRIP / 2}
                dy="0.32em"
                textAnchor="end"
                className="fill-[var(--ink-2)] text-[10px]"
              >
                Δ%
              </text>
              {changes.map((one) => {
                const height = Math.max((Math.abs(one.percent) / reach) * 22, 1);
                const rising = one.percent >= 0;
                return (
                  <g key={one.seconds}>
                    <rect
                      x={x(one.seconds) - 6}
                      y={rising ? STRIP / 2 - height : STRIP / 2}
                      width={12}
                      height={height}
                      rx={2}
                      fill={rising ? "var(--good)" : "var(--alert)"}
                    />
                    <text
                      x={x(one.seconds) + 9}
                      y={rising ? STRIP / 2 - 6 : STRIP / 2 + 12}
                      className="fill-[var(--ink-2)] text-[10px] tabular-nums"
                    >
                      {signed(one.percent, 1)}%
                    </text>
                  </g>
                );
              })}
            </svg>
          ) : null}
        </div>
        <table className="w-full text-sm tabular-nums">
          <thead className="text-[var(--ink-2)] text-xs">
            <tr>
              <th className="text-left font-normal">Best</th>
              <th className="text-right font-normal">Watts</th>
              {previous ? <th className="text-right font-normal">Change</th> : null}
            </tr>
          </thead>
          <tbody>
            {current.map((one) => {
              const earlier = before.get(one.seconds);
              return (
                <tr key={one.seconds}>
                  <td>{formatCurveDuration(one.seconds)}</td>
                  <td className="text-right">{Math.round(one.watts)} W</td>
                  {previous ? (
                    <td className="text-right text-[var(--ink-2)]">
                      {earlier === undefined ? "—" : signed(one.watts - earlier)}
                    </td>
                  ) : null}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </>
  );
}
