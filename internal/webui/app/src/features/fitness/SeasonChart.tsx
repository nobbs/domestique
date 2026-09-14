/**
 * The season in three stacked panels — fitness, form, weekly load — running
 * straight on into the three weeks the service projected under three plans.
 *
 * Form is drawn as a percentage of fitness against its bands, so the panel
 * reads the same whichever scale the page is on.
 */

import type { FitnessPlan, FitnessScaleOutlook } from "../../api/types";
import {
  ChartLegend,
  FRAME_LEFT,
  type FramePanel,
  formatCalendarDay,
  ReadoutRow,
  TimeFrame,
} from "../../components/chart/TimeFrame";
import {
  bandOf,
  bandTop,
  daysBetween,
  FORM_BANDS,
  formPercent,
  mondayOf,
  type Reading,
  signed,
  weeklyLoads,
} from "./form";

export const FITNESS_COLOUR = "var(--accent)";
const FATIGUE_COLOUR = "var(--hold)";
const FORM_COLOUR = "var(--ink)";

export const PLANS: Record<FitnessPlan["plan"], { name: string; colour: string }> = {
  rest: { name: "Rest", colour: "var(--good)" },
  habitual: { name: "As the last 4 weeks", colour: FITNESS_COLOUR },
  build: { name: "A fifth more", colour: FATIGUE_COLOUR },
};

/** Room the projection takes whatever the range, so three weeks stay legible beside a year. */
const FUTURE_SHARE = 0.22;
// Clamped, so one event's crater doesn't squash every band into a sliver.
const FORM_FLOOR = -55;
const FORM_CEILING = 40;

const points = (
  values: readonly number[],
  x: (index: number) => number,
  y: (value: number) => number,
  offset = 0,
) => values.map((value, index) => `${x(index + offset)},${y(value)}`).join(" ");

interface Props {
  readings: readonly Reading[];
  outlook: FitnessScaleOutlook | undefined;
  scaleName: string;
}

export function SeasonChart({ readings, outlook, scaleName }: Props) {
  const today = readings[readings.length - 1];
  if (!today) {
    return null;
  }
  const plans = outlook?.plans ?? [];
  const projected = plans[0]?.days ?? [];
  const todayIndex = readings.length - 1;
  const dates = [...readings.map((one) => one.date), ...projected.map((one) => one.date)];
  const indexOf = new Map(dates.map((date, index) => [date, index]));
  const peak = readings.reduce((best, one) => (one.fitness > best.fitness ? one : best), today);
  const weeks = weeklyLoads(readings);
  const thisMonday = mondayOf(today.date);

  const futureForm = plans.map((plan) =>
    plan.days.map((one) => formPercent(one.form, one.fitness)),
  );
  const allForm = [...readings.map((one) => one.formPercent), ...futureForm.flat()];
  const formLow = Math.max(Math.min(-40, ...allForm) - 5, FORM_FLOOR);
  const formHigh = Math.min(Math.max(25, ...allForm) + 5, FORM_CEILING);
  const clampForm = (value: number) => Math.min(Math.max(value, formLow), formHigh);
  const fitnessHigh =
    Math.max(
      1,
      ...readings.map((one) => one.fitness),
      ...plans.flatMap((plan) => plan.days.map((one) => one.fitness)),
    ) * 1.15;
  const loadHigh = Math.max(1, outlook?.weekLoadHigh ?? 0, ...weeks.values()) * 1.05;

  const tails = (values: (planIndex: number) => number[], start: number) =>
    function drawTails(x: (index: number) => number, y: (value: number) => number) {
      return plans.map((plan, planIndex) => {
        const series = [start, ...values(planIndex)];
        return (
          <g key={plan.plan}>
            <polyline
              points={points(series, x, y, todayIndex)}
              fill="none"
              stroke={PLANS[plan.plan].colour}
              strokeWidth={2}
              strokeDasharray="5 3"
            />
            <circle
              cx={x(dates.length - 1)}
              cy={y(series[series.length - 1] ?? 0)}
              r={3}
              fill={PLANS[plan.plan].colour}
            />
          </g>
        );
      });
    };

  const weekBox = (x: (index: number) => number, monday: string) => {
    // A week that began before the range is drawn over the days of it inside the range.
    const start = daysBetween(dates[0] ?? monday, monday);
    const left = x(Math.max(start, 0)) + 1;
    const right = x(Math.min(start + 7, dates.length - 1)) - 1;

    return { left, width: Math.max(right - left, 1) };
  };

  const panels: FramePanel[] = [
    {
      height: 160,
      title: "Fitness",
      domain: [0, fitnessHigh],
      draw: (x, y) => {
        const values = readings.map((one) => one.fitness);
        return (
          <>
            <polygon
              points={`${x(0)},${y(0)} ${points(values, x, y)} ${x(todayIndex)},${y(0)}`}
              fill={FITNESS_COLOUR}
              opacity={0.1}
            />
            <polyline
              points={points(values, x, y)}
              fill="none"
              stroke={FITNESS_COLOUR}
              strokeWidth={2}
            />
            <circle
              cx={x(indexOf.get(peak.date) ?? 0)}
              cy={y(peak.fitness)}
              r={4}
              fill={FITNESS_COLOUR}
              stroke="var(--panel)"
              strokeWidth={2}
            />
            {tails(
              (planIndex) => plans[planIndex]?.days.map((one) => one.fitness) ?? [],
              today.fitness,
            )(x, y)}
          </>
        );
      },
    },
    {
      height: 130,
      title: "Form, % of fitness",
      domain: [formLow, formHigh],
      ticks: [-30, 0, 20].filter((tick) => tick > formLow && tick < formHigh),
      format: (value) => `${value > 0 ? "+" : ""}${value}`,
      draw: (x, y) => {
        const clamped = (value: number) => y(clampForm(value));
        const right = x(dates.length - 1);
        return (
          <>
            {FORM_BANDS.map((band) => {
              const top = y(Math.min(bandTop(FORM_BANDS, band), formHigh));
              const bottom = y(Math.max(band.from, formLow));
              if (bottom <= top) {
                return null;
              }
              return (
                <g key={band.name}>
                  <rect
                    x={FRAME_LEFT}
                    y={top}
                    width={right - FRAME_LEFT}
                    height={bottom - top}
                    fill={band.name === "Grey zone" ? "transparent" : band.colour}
                    opacity={0.12}
                  />
                  {bottom - top < 11 ? null : (
                    <text
                      x={right - 4}
                      y={(top + bottom) / 2}
                      dy="0.32em"
                      textAnchor="end"
                      className="fill-[var(--ink-2)] text-[10px]"
                      stroke="var(--panel)"
                      strokeWidth={3}
                      paintOrder="stroke"
                    >
                      {band.name}
                    </text>
                  )}
                </g>
              );
            })}
            <polyline
              points={points(
                readings.map((one) => one.formPercent),
                x,
                clamped,
              )}
              fill="none"
              stroke={FORM_COLOUR}
              strokeWidth={1.5}
            />
            {tails((planIndex) => futureForm[planIndex] ?? [], today.formPercent)(x, clamped)}
          </>
        );
      },
    },
    {
      height: 70,
      title: "Weekly load",
      domain: [0, loadHigh],
      ticks: [0, Math.round(loadHigh / 100) * 100].filter((tick, index) => index === 0 || tick > 0),
      draw: (x, y) => (
        <>
          {[...weeks].map(([monday, load]) => {
            const box = weekBox(x, monday);
            return load > 0 ? (
              <rect
                key={monday}
                x={box.left}
                y={y(load)}
                width={box.width}
                height={y(0) - y(load)}
                rx={2}
                fill="var(--ink-2)"
                opacity={0.45}
              />
            ) : null;
          })}
          {outlook ? (
            <rect
              x={weekBox(x, thisMonday).left}
              y={y(outlook.weekLoadHigh)}
              width={weekBox(x, thisMonday).width}
              height={Math.max(y(outlook.weekLoadLow) - y(outlook.weekLoadHigh), 1)}
              rx={2}
              fill="var(--good)"
              opacity={0.25}
              stroke="var(--good)"
              strokeDasharray="3 2"
            />
          ) : null}
        </>
      ),
    },
  ];

  const readout = (index: number) => {
    if (index <= todayIndex) {
      const one = readings[index];
      if (!one) {
        return null;
      }
      const monday = mondayOf(one.date);
      return (
        <>
          <ReadoutRow colour={FITNESS_COLOUR} label="fitness" value={one.fitness.toFixed(0)} />
          <ReadoutRow colour={FATIGUE_COLOUR} label="fatigue" value={one.fatigue.toFixed(0)} />
          <ReadoutRow
            colour={FORM_COLOUR}
            label={`form · ${bandOf(FORM_BANDS, one.formPercent).name}`}
            value={`${signed(one.form)} (${signed(one.formPercent)}%)`}
          />
          <ReadoutRow label="load that day" value={one.load.toFixed(0)} />
          <ReadoutRow
            label={`week of ${formatCalendarDay(monday)}`}
            value={(weeks.get(monday) ?? 0).toFixed(0)}
          />
        </>
      );
    }
    const ahead = index - todayIndex - 1;
    return (
      <>
        <div className="mb-1 text-[var(--ink-2)]">Projected form</div>
        {plans.map((plan, planIndex) => {
          const percent = futureForm[planIndex]?.[ahead];
          return percent === undefined ? null : (
            <ReadoutRow
              key={plan.plan}
              colour={PLANS[plan.plan].colour}
              dashed
              label={`${PLANS[plan.plan].name} · ${bandOf(FORM_BANDS, percent).name}`}
              value={`${signed(percent)}%`}
            />
          );
        })}
      </>
    );
  };

  return (
    <>
      <ChartLegend
        items={[
          { label: "Fitness", colour: FITNESS_COLOUR },
          { label: "Form", colour: FORM_COLOUR },
          ...plans.map((plan) => ({
            label: PLANS[plan.plan].name,
            colour: PLANS[plan.plan].colour,
            dashed: true,
          })),
        ]}
      />
      <TimeFrame
        label={`Fitness, form and weekly load over ${readings.length} days on the ${scaleName} scale${
          plans.length > 0
            ? `, and ${projected.length} days projected under ${plans.length} plans`
            : ""
        }`}
        dates={dates}
        panels={panels}
        readout={readout}
        {...(plans.length > 0 ? { todayIndex, futureShare: FUTURE_SHARE } : {})}
      />
      {outlook ? (
        <p className="text-[var(--ink-2)] text-xs">
          Right of the dotted line, the next three weeks are drawn wider than the past. Each plan
          spreads its load evenly over every day; the green box is this week's load that would raise
          fitness 3–8%.
        </p>
      ) : null}
    </>
  );
}
