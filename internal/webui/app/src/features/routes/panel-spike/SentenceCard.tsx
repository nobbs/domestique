/**
 * **C — Sentence.** The route described rather than tabulated.
 *
 * A grid of five figures is read five times: the eye lands on a number, finds
 * its label, decides whether it matters, moves on. A rider asking whether a
 * route is the one for Saturday is not comparing columns — they are forming a
 * judgement, and a judgement is what prose delivers in one pass. "Mostly
 * asphalt with a gravel col in the middle" is a thing a person says; `gravel
 * 10%` is a thing a person has to interpret.
 *
 * Every figure in the paragraphs is generated from the same measurements the
 * other alternatives tabulate — nothing here is written by hand — and every
 * class name is the control it would have been as a chip, underlined in its
 * own colour so the palette survives the prose. Beneath sits one fused ribbon,
 * steepness over ground, as the picture the sentences point at.
 *
 * The risk is honest and worth watching for: prose is bad at being scanned. A
 * reader flicking between four routes for the one with the least climbing has
 * to read four paragraphs rather than glance down one column, and the pill —
 * which carries distance and ascent — is doing that job instead.
 */

import type { ReactNode } from "react";
import type { SurfaceKind } from "../../../api/types";
import {
  formatAscent,
  formatDistance,
  formatGradient,
  formatMovingTime,
} from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import { GRADIENT_BANDS } from "../../../lib/profile";
import { SURFACE_STYLES } from "../../../lib/surface";
import type { CardProps } from "./shared";
import {
  bandLabel,
  bandVariable,
  CardHeading,
  formatShare,
  gradientSegments,
  groundSegments,
  HighlightToggle,
  Ribbon,
  surfaceLabel,
  surfaceVariable,
} from "./shared";

/** Small counts read as words in prose; large ones stay figures. */
const NUMBER_WORDS = ["no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"];

function spell(count: number): string {
  return NUMBER_WORDS[count] ?? `${count}`;
}

/**
 * A share as the phrase a person would use, or as a figure when none fits.
 *
 *
 * "A fifth of it is steep" is a fact a reader keeps; "21.4% of it is steep" is
 * a fact a reader has to convert first. The tolerance is deliberately tight —
 * a third that is really 28% would be the card lying to sound natural.
 */
const FRACTIONS: ReadonlyArray<readonly [number, string]> = [
  [1 / 2, "half"],
  [1 / 3, "a third"],
  [2 / 3, "two thirds"],
  [1 / 4, "a quarter"],
  [3 / 4, "three quarters"],
  [1 / 5, "a fifth"],
  [2 / 5, "two fifths"],
  [3 / 5, "three fifths"],
  [1 / 10, "a tenth"],
];

function approximately(share: number): string {
  const nearest = FRACTIONS.find(([value]) => Math.abs(share - value) <= 0.03);

  return nearest ? nearest[1] : formatShare(share);
}

/** A class name, pressable, underlined in the colour it is drawn in elsewhere. */
function Token({
  highlight,
  current,
  onChange,
  colour,
  label,
  description,
  children,
}: {
  highlight: Highlight;
  current: Highlight | null;
  onChange: (next: Highlight | null) => void;
  colour: string;
  label: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <HighlightToggle
      highlight={highlight}
      current={current}
      onChange={onChange}
      label={`${label}, ${description}`}
      title={description}
      className="rounded-[3px] px-0.5 font-semibold decoration-2 underline-offset-[3px] hover:bg-[var(--base)] aria-pressed:bg-[color-mix(in_srgb,var(--accent)_16%,transparent)]"
      style={{ textDecorationLine: "underline", textDecorationColor: colour }}
    >
      {children}
    </HighlightToggle>
  );
}

/** `a, b and c` — the join a sentence needs and `Array.join` cannot do. */
function series(parts: ReactNode[]): ReactNode[] {
  return parts.flatMap((part, index) => {
    if (index === 0) {
      return [part];
    }
    const separator = index === parts.length - 1 ? " and " : ", ";

    return [separator, part];
  });
}

export function SentenceCard({
  route,
  subtitle,
  movingSeconds,
  bands,
  runs,
  surface,
  surfaceAbsence,
  climbs,
  highlight,
  onHighlightChange,
  unitSystem,
}: CardProps) {
  const biggest = climbs.reduce<(typeof climbs)[number] | null>(
    (worst, climb) => (worst === null || climb.ascentMetres > worst.ascentMetres ? climb : worst),
    null,
  );

  // Bands worth naming, steepest first: a rider reads a route from its hardest
  // ground down, and the gentle share is what is left over rather than what is
  // asked about.
  const steepFirst = [...bands].sort((left, right) => right.band - left.band);
  const named = steepFirst.filter((entry) => entry.share >= 0.03);
  const remainder = steepFirst
    .filter((entry) => entry.share < 0.03)
    .reduce((sum, entry) => sum + entry.share, 0);
  const steepest = steepFirst[0];

  const surfaces = [...(surface?.shares ?? [])].sort((left, right) => right.share - left.share);
  const dominant = surfaces[0];
  const notable = surfaces.slice(1).filter((entry) => entry.share >= 0.05);
  const traces = surfaces.slice(1).filter((entry) => entry.share < 0.05);

  const groundToken = (kind: SurfaceKind, text: string) => (
    <Token
      key={kind}
      highlight={{ type: "surface", kind }}
      current={highlight}
      onChange={onHighlightChange}
      colour={surfaceVariable(kind)}
      label={surfaceLabel(kind)}
      description={SURFACE_STYLES[kind].description}
    >
      {text}
    </Token>
  );

  return (
    <div className="grid gap-3">
      <CardHeading route={route} subtitle={subtitle} />
      <div className="grid gap-2 text-sm leading-relaxed">
        <p>
          <span className="font-semibold tabular-nums">
            {formatDistance(route.distanceMetres, unitSystem)}
          </span>{" "}
          and{" "}
          <span className="font-semibold tabular-nums">
            {formatAscent(route.ascentMetres, unitSystem)}
          </span>{" "}
          of climbing, about{" "}
          <span className="font-semibold tabular-nums">{formatMovingTime(movingSeconds)}</span>{" "}
          moving.
          {biggest === null ? null : (
            <>
              {" "}
              {spell(climbs.length).replace(/^\w/, (first) => first.toUpperCase())}{" "}
              {climbs.length === 1 ? "sustained climb" : "sustained climbs"}, the biggest{" "}
              <span className="font-semibold tabular-nums">
                {formatDistance(biggest.distanceMetres, unitSystem)}
              </span>{" "}
              at{" "}
              <span className="font-semibold tabular-nums">
                {formatGradient(biggest.averageGradePercent)}
              </span>
              .
            </>
          )}
        </p>
        {named.length === 0 ? (
          <p className="text-[var(--ink-2)]">No elevation data.</p>
        ) : (
          <p>
            It is{" "}
            {series(
              named.map((entry) => (
                <Token
                  key={entry.band}
                  highlight={{ type: "band", band: entry.band }}
                  current={highlight}
                  onChange={onHighlightChange}
                  colour={bandVariable(entry.band)}
                  label={`${bandLabel(entry.band)} gradient`}
                  description={GRADIENT_BANDS[entry.band]?.description ?? ""}
                >
                  {`${GRADIENT_BANDS[entry.band]?.description ?? ""} for ${approximately(entry.share)}`}
                </Token>
              )),
            )}
            {remainder > 0 && steepest ? (
              <>
                , and steeper still for{" "}
                <Token
                  highlight={{ type: "band", band: steepest.band }}
                  current={highlight}
                  onChange={onHighlightChange}
                  colour={bandVariable(steepest.band)}
                  label={`${bandLabel(steepest.band)} gradient`}
                  description={GRADIENT_BANDS[steepest.band]?.description ?? ""}
                >
                  {formatShare(remainder)}
                </Token>
              </>
            ) : null}
            {`. It touches ${formatGradient(route.maxGradientPercent)}.`}
          </p>
        )}
        {dominant === undefined ? (
          <p className="text-[var(--ink-2)]">{surfaceAbsence}</p>
        ) : (
          <p>
            {dominant.share >= 0.5 ? "Mostly " : "Mainly "}
            {groundToken(dominant.kind, surfaceLabel(dominant.kind).toLowerCase())}
            {notable.length === 0 ? "" : ", with "}
            {notable.length === 0
              ? null
              : series(
                  notable.map((entry) =>
                    groundToken(
                      entry.kind,
                      `${formatShare(entry.share)} ${surfaceLabel(entry.kind).toLowerCase()}`,
                    ),
                  ),
                )}
            {"."}
            {traces.length === 0 ? null : (
              <>
                {" A little "}
                {series(
                  traces.map((entry) =>
                    groundToken(entry.kind, surfaceLabel(entry.kind).toLowerCase()),
                  ),
                )}
                {"."}
              </>
            )}
          </p>
        )}
      </div>
      {/*
       * One ribbon rather than two, steepness sitting directly on ground: the
       * sentences have already said how much of each there is, so all this has
       * left to say is where — and fused, it says where they coincide, which is
       * the gravel col the prose can only mention twice.
       */}
      <div className="grid gap-px">
        <Ribbon segments={gradientSegments(runs)} className="h-2.5" highlight={highlight} />
        <Ribbon segments={groundSegments(surface)} className="h-2" highlight={highlight} />
      </div>
    </div>
  );
}
