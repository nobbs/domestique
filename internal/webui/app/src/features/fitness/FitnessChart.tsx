/**
 * Fitness, fatigue and form over time.
 *
 * Three lines and a row of bars in one frame, because they are one story: the
 * daily load is what happened, fitness is what it accumulated into, fatigue is
 * what it cost, and form is what is left over. Reading them apart would mean
 * reading three charts against each other.
 *
 * Form is the one that crosses zero, so it gets the zero rule and is drawn last,
 * over the two averages it is the difference of.
 */

import { useId } from "react";
import type { FitnessDay } from "../../api/types";

const HEIGHT = 220;
const WIDTH = 720;
const PADDING = { top: 12, right: 8, bottom: 20, left: 36 };

/** Which of the two scales the chart is reading. */
export type Scale = "tss" | "trimp";

interface Reading {
  load: number;
  fitness: number;
  fatigue: number;
  form: number;
}

/** One day on the chosen scale, which is all the drawing needs to know. */
function reading(day: FitnessDay, scale: Scale): Reading {
  return scale === "tss"
    ? { load: day.tssLoad, fitness: day.tssFitness, fatigue: day.tssFatigue, form: day.tssForm }
    : {
        load: day.trimpLoad,
        fitness: day.trimpFitness,
        fatigue: day.trimpFatigue,
        form: day.trimpForm,
      };
}

/** A polyline through one series, in the frame's own coordinates. */
function path(
  values: number[],
  x: (index: number) => number,
  y: (value: number) => number,
): string {
  return values.map((value, index) => `${x(index)},${y(value)}`).join(" ");
}

export function FitnessChart({ days, scale }: { days: FitnessDay[]; scale: Scale }) {
  const titleId = useId();
  if (days.length === 0) {
    return null;
  }
  const readings = days.map((day) => reading(day, scale));
  const highest = Math.max(...readings.map((one) => Math.max(one.load, one.fitness)), 1);
  // Form runs below zero, so the frame has to hold the deepest of it.
  const lowest = Math.min(...readings.map((one) => one.form), 0);
  const span = highest - lowest;

  const plotWidth = WIDTH - PADDING.left - PADDING.right;
  const plotHeight = HEIGHT - PADDING.top - PADDING.bottom;
  const x = (index: number) =>
    PADDING.left + (days.length === 1 ? plotWidth / 2 : (index / (days.length - 1)) * plotWidth);
  const y = (value: number) => PADDING.top + plotHeight - ((value - lowest) / span) * plotHeight;
  const barWidth = Math.max(plotWidth / days.length - 1, 1);

  return (
    <svg
      viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
      className="w-full"
      role="img"
      aria-labelledby={titleId}
      preserveAspectRatio="none"
    >
      <title id={titleId}>
        Fitness, fatigue and form over {days.length} days, on the{" "}
        {scale === "tss" ? "stress score" : "TRIMP"} scale
      </title>
      {/* Zero is the line form is read against: above it fresh, below it buried. */}
      <line
        x1={PADDING.left}
        x2={WIDTH - PADDING.right}
        y1={y(0)}
        y2={y(0)}
        stroke="var(--rule)"
        strokeWidth={1}
      />
      {readings.map((one, index) =>
        one.load > 0 ? (
          <rect
            key={days[index]?.date}
            x={x(index) - barWidth / 2}
            y={y(one.load)}
            width={barWidth}
            height={Math.max(y(0) - y(one.load), 1)}
            fill="var(--rule)"
          />
        ) : null,
      )}
      <polyline
        points={path(
          readings.map((one) => one.fatigue),
          x,
          y,
        )}
        fill="none"
        stroke="var(--hold)"
        strokeWidth={1.5}
      />
      <polyline
        points={path(
          readings.map((one) => one.fitness),
          x,
          y,
        )}
        fill="none"
        stroke="var(--accent)"
        strokeWidth={2}
      />
      <polyline
        points={path(
          readings.map((one) => one.form),
          x,
          y,
        )}
        fill="none"
        stroke="var(--ink-2)"
        strokeWidth={1.5}
        strokeDasharray="4 3"
      />
    </svg>
  );
}
