/**
 * The sideways card: figures in a block, both mixes as upright bars beside it.
 *
 * Shared by two alternatives that differ in exactly one thing — whether a
 * class is labelled with its share of the route or with how much ground it
 * actually covers. That difference is the experiment, so the layout around it
 * is held here rather than copied into each, where it could drift and quietly
 * turn a comparison of figures into a comparison of two cards.
 *
 * The arrangement itself: the other alternatives read down the page, which is
 * the direction the panel cannot afford — it stands on a map, and every row it
 * adds is a row of terrain the reader came to look at. Turning it ninety
 * degrees comes out around a third the height of the ledger for the same
 * content.
 *
 * The bars are read like a cross-section, with the ground that decides the day
 * at the top and the easy majority settling at the bottom. Labels sit beside
 * their own segment rather than in a row underneath, so nothing has to be
 * matched by colour — the name is at the height of the thing it names.
 *
 * That alignment is the whole trick, and the naive form of it destroys the
 * thing it decorates: a class covering one percent gets a segment two pixels
 * tall, and flooring the rows so its name fits makes every class the same
 * height — a bar that is no longer a proportion. So the bar is left exactly
 * proportional and the labels are placed beside it, pushed apart only where
 * they would collide, with a leader line drawn to the segment each one names.
 */

import {
  formatAscent,
  formatDistance,
  formatElevation,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
} from "../../../lib/format";
import { sameHighlight } from "../../../lib/highlight";
import type { UnitSystem } from "../../../lib/units";
import type { CardProps, MixEntry } from "./shared";
import {
  bandEntries,
  CardHeading,
  climbSentence,
  formatShare,
  HighlightToggle,
  surfaceEntries,
} from "./shared";

/**
 * What a class is labelled with — the one thing the two alternatives disagree on.
 *
 * The whole column is handed over, not just the row: a figure that reads as a
 * stack has to be scaled by the stack rather than by itself, or the largest
 * number in it can end up naming the shortest stretch.
 */
export type MixFigure = (entry: MixEntry, unitSystem: UnitSystem, column: MixEntry[]) => string;

const COLUMN_HEIGHT = 132;
/** A name at eleven pixels, with enough air not to touch the one below it. */
const LABEL_HEIGHT = 17;
const BAR_WIDTH = 8;
const LEADER_WIDTH = 12;

/**
 * Where each label goes, given where its segment is.
 *
 * The obvious arrangement — give the label the same height as its segment —
 * is the one that quietly destroys the bar: floor the rows so a one percent
 * class can still be read and every class ends up the same height, at which
 * point the bar has stopped being a proportion and become a legend drawn
 * vertically.
 *
 * So the bar stays exactly proportional and the labels are placed beside it
 * instead. Each wants to sit level with the middle of its own segment; where
 * two would overlap, the lower one is pushed down and a leader line keeps the
 * correspondence explicit. Six classes at seventeen pixels fit inside the
 * column with room to spare, so the pass never runs out of space.
 */
function placeLabels(shares: number[]): { middle: number; top: number }[] {
  let offset = 0;
  const middles = shares.map((share) => {
    const middle = offset + (share * COLUMN_HEIGHT) / 2;
    offset += share * COLUMN_HEIGHT;

    return middle;
  });

  const tops: number[] = [];
  let lowest = 0;
  for (const middle of middles) {
    const top = Math.max(middle - LABEL_HEIGHT / 2, lowest);
    tops.push(top);
    lowest = top + LABEL_HEIGHT;
  }

  // The downward pass can push the last label off the bottom. Walking back up
  // from the floor gives every label the highest position it can hold without
  // landing on the one below it.
  if (lowest > COLUMN_HEIGHT) {
    let ceiling = COLUMN_HEIGHT;
    for (let index = tops.length - 1; index >= 0; index--) {
      const top = Math.max(Math.min(tops[index] ?? 0, ceiling - LABEL_HEIGHT), 0);
      tops[index] = top;
      ceiling = top;
    }
  }

  return middles.map((middle, index) => ({ middle, top: tops[index] ?? middle }));
}

function Figure({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-[11px] leading-none text-[var(--ink-2)]">{term}</dt>
      <dd className="text-sm leading-tight tabular-nums">{children}</dd>
    </div>
  );
}

function StackedColumn({
  name,
  entries,
  absence,
  figure,
  highlight,
  onHighlightChange,
  unitSystem,
}: {
  name: string;
  /** Canonical order, gentlest and smoothest first — reversed on the way in. */
  entries: MixEntry[];
  absence: string | null;
  figure: MixFigure;
  highlight: CardProps["highlight"];
  onHighlightChange: CardProps["onHighlightChange"];
  unitSystem: UnitSystem;
}) {
  const stacked = [...entries].reverse();
  const places = placeLabels(stacked.map((entry) => entry.share));

  return (
    <section className="min-w-0 flex-1">
      <h3 className="mb-1 text-[11px] font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
        {name}
      </h3>
      {stacked.length === 0 ? (
        <p className="text-xs text-[var(--ink-2)]">{absence}</p>
      ) : (
        <div className="relative" style={{ height: COLUMN_HEIGHT }}>
          <div
            className="absolute inset-y-0 left-0 flex flex-col overflow-hidden rounded-sm"
            style={{ width: BAR_WIDTH }}
            aria-hidden="true"
          >
            {stacked.map((entry) => (
              <span
                key={entry.label}
                style={{
                  flexGrow: entry.share,
                  flexBasis: 0,
                  background: entry.colour,
                  opacity:
                    highlight === null || sameHighlight(highlight, entry.highlight) ? 1 : 0.2,
                }}
              />
            ))}
          </div>
          <svg
            className="absolute inset-y-0 overflow-visible"
            style={{ left: BAR_WIDTH, width: LEADER_WIDTH }}
            viewBox={`0 0 ${LEADER_WIDTH} ${COLUMN_HEIGHT}`}
            aria-hidden="true"
          >
            {stacked.map((entry, index) => {
              const place = places[index];
              if (!place) {
                return null;
              }
              const to = place.top + LABEL_HEIGHT / 2;

              return (
                <path
                  key={entry.label}
                  d={`M0 ${place.middle.toFixed(1)} H${LEADER_WIDTH / 2} V${to.toFixed(1)} H${LEADER_WIDTH}`}
                  className="stroke-[var(--rule)]"
                  strokeWidth={1}
                  fill="none"
                />
              );
            })}
          </svg>
          {stacked.map((entry, index) => (
            <div
              key={entry.label}
              className="absolute right-0"
              style={{
                left: BAR_WIDTH + LEADER_WIDTH,
                top: places[index]?.top ?? 0,
                height: LABEL_HEIGHT,
              }}
            >
              <HighlightToggle
                highlight={entry.highlight}
                current={highlight}
                onChange={onHighlightChange}
                // Both quantities spoken whichever one is drawn: the figure on
                // screen is a choice about space, not about what the class is.
                label={`${entry.label}, ${entry.description}, ${formatDistance(entry.metres, unitSystem)}, ${formatShare(entry.share)} of the route`}
                title={entry.description}
                className="flex size-full min-w-0 items-center gap-1 rounded px-1 text-left text-[11px] leading-none hover:bg-[var(--base)] aria-pressed:bg-[color-mix(in_srgb,var(--accent)_16%,transparent)]"
              >
                <span className="truncate text-[var(--ink-2)]">{entry.label}</span>
                <span className="ml-auto shrink-0 font-semibold tabular-nums">
                  {figure(entry, unitSystem, stacked)}
                </span>
              </HighlightToggle>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

export function SidewaysCard({
  figure,
  route,
  subtitle,
  movingSeconds,
  highestMetres,
  bands,
  surface,
  surfaceAbsence,
  climbs,
  highlight,
  onHighlightChange,
  unitSystem,
}: CardProps & { figure: MixFigure }) {
  const climbLine = climbSentence(climbs, unitSystem);

  return (
    <div className="grid gap-2">
      <CardHeading route={route} subtitle={subtitle} />
      <div className="flex items-start gap-3">
        <dl className="grid w-[9.5rem] shrink-0 grid-cols-2 gap-x-3 gap-y-1.5">
          <Figure term="Distance">{formatDistance(route.distanceMetres, unitSystem)}</Figure>
          <Figure term="Ascent">{formatAscent(route.ascentMetres, unitSystem)}</Figure>
          <Figure term="Max gradient">{formatGradient(route.maxGradientPercent)}</Figure>
          <Figure term="Highest">
            {highestMetres === null ? "—" : formatElevation(highestMetres, unitSystem)}
          </Figure>
          <div className="col-span-2">
            <dt className="text-[11px] leading-none text-[var(--ink-2)]">Moving time</dt>
            <dd className="text-sm leading-tight tabular-nums">
              {formatMovingTime(movingSeconds)}
              {movingSeconds !== undefined && route.validation ? (
                <span className="ml-1 text-[11px] text-[var(--ink-2)]">
                  {formatMovingTimeUncertainty(route.validation)}
                </span>
              ) : null}
            </dd>
          </div>
        </dl>
        <StackedColumn
          name="Gradient"
          entries={bandEntries(bands, route.distanceMetres)}
          absence="No elevation data."
          figure={figure}
          highlight={highlight}
          onHighlightChange={onHighlightChange}
          unitSystem={unitSystem}
        />
        <StackedColumn
          name="Surface"
          entries={surfaceEntries(surface)}
          absence={surfaceAbsence}
          figure={figure}
          highlight={highlight}
          onHighlightChange={onHighlightChange}
          unitSystem={unitSystem}
        />
      </div>
      {climbLine === null ? null : (
        <p className="border-t border-[var(--rule)] pt-2 text-xs text-[var(--ink-2)]">
          {climbLine}
        </p>
      )}
    </div>
  );
}
