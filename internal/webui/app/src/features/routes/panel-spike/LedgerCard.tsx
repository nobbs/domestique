/**
 * **A — Ledger.** The route as a spec sheet, and the bar and the chips fused.
 *
 * Every figure on one hairline-ruled column, label left and value right, so
 * the card is read down a single edge rather than scanned across a grid. It is
 * the most conservative arrangement here and the easiest to compare two routes
 * with, because every value starts at the same x.
 *
 * The one real idea is underneath. The legend today draws a proportion bar and
 * then a row of chips that repeat its segments — two objects saying the same
 * thing, which cost two rows and force the eye to match colours between them.
 * Here the chips *are* the bar: each one is as wide as its share of the route,
 * laid edge to edge, so the row is a bar you can read the words off and press.
 *
 * The cost is that a class covering under a couple of percent cannot hold its
 * own name. Those chips are floored at a width that keeps them pressable and
 * legible, which makes the row very slightly wider than proportional — the
 * bar lies by a pixel or two rather than hiding a class the route has.
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
import { useElementWidth } from "../../../lib/useElementWidth";
import type { CardProps, MixEntry } from "./shared";
import {
  bandEntries,
  CardHeading,
  climbSentence,
  formatShare,
  HighlightToggle,
  surfaceEntries,
} from "./shared";

/** Narrow enough to be a sliver, wide enough to press and to say a figure. */
const MINIMUM_CHIP = "2.75rem";

/**
 * Whether a chip that wide can hold that name.
 *
 * A share threshold cannot answer this: "flat" fits in a chip that "Compacted"
 * does not, and both are drawn from the same percentages. So the row is
 * measured and the name is dropped when it would be clipped — a chip reading
 * `Com…` names nothing and still spends a word's width doing it, while the
 * figure alone is exact, and the colour and the tooltip still carry the class.
 *
 * This is the honest limit of fusing the bar and the chips: a route made of
 * six kinds of ground cannot say all six on one bar's width. Six pixels an
 * character is a deliberate over-estimate at eleven pixels, so the rule errs
 * towards dropping a name that would have just fitted rather than clipping one.
 */
function fitsName(share: number, name: string, rowWidth: number): boolean {
  return share * rowWidth >= name.length * 6 + 16;
}

function Row({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-b border-dotted border-[var(--rule)] py-1 last:border-b-0">
      <dt className="text-[var(--ink-2)]">{term}</dt>
      <dd className="text-right tabular-nums">{children}</dd>
    </div>
  );
}

function ProportionChips({
  name,
  entries,
  absence,
  highlight,
  onHighlightChange,
}: {
  name: string;
  entries: MixEntry[];
  absence: string | null;
  highlight: CardProps["highlight"];
  onHighlightChange: CardProps["onHighlightChange"];
}) {
  const { ref, width } = useElementWidth<HTMLUListElement>();

  return (
    <section>
      <h3 className="mb-1 text-xs font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
        {name}
      </h3>
      {entries.length === 0 ? (
        <p className="text-sm text-[var(--ink-2)]">{absence}</p>
      ) : (
        <ul ref={ref} className="flex gap-px overflow-hidden rounded-md">
          {entries.map((entry) => {
            const pressed = sameHighlight(highlight, entry.highlight);

            return (
              <li
                key={entry.label}
                className="min-w-0"
                style={{ flexGrow: entry.share, flexBasis: 0, minWidth: MINIMUM_CHIP }}
              >
                <HighlightToggle
                  highlight={entry.highlight}
                  current={highlight}
                  onChange={onHighlightChange}
                  label={`${entry.label}, ${entry.description}, ${formatShare(entry.share)} of the route`}
                  title={entry.description}
                  className="flex h-10 w-full flex-col justify-center overflow-hidden px-1.5 text-left"
                  style={{
                    // A wash of the class's own colour with a solid edge, so a
                    // row of them reads as a bar at a glance and as labelled
                    // controls up close. Pressing deepens the wash rather than
                    // outlining it: an outline on a segment butted against its
                    // neighbours reads as a gap.
                    background: `color-mix(in srgb, ${entry.colour} ${pressed ? 55 : 20}%, transparent)`,
                    boxShadow: `inset 3px 0 0 ${entry.colour}`,
                  }}
                >
                  {fitsName(entry.share, entry.label, width) ? (
                    <span className="truncate text-[11px] text-[var(--ink-2)]">{entry.label}</span>
                  ) : null}
                  <span className="truncate text-xs font-semibold tabular-nums">
                    {formatShare(entry.share)}
                  </span>
                </HighlightToggle>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

export function LedgerCard({
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
}: CardProps) {
  const climbLine = climbSentence(climbs, unitSystem);

  return (
    <div className="grid gap-3">
      <CardHeading route={route} subtitle={subtitle} />
      <dl className="text-sm">
        <Row term="Distance">{formatDistance(route.distanceMetres, unitSystem)}</Row>
        <Row term="Ascent">{formatAscent(route.ascentMetres, unitSystem)}</Row>
        <Row term="Max gradient">{formatGradient(route.maxGradientPercent)}</Row>
        <Row term="Moving time">
          {formatMovingTime(movingSeconds)}
          {movingSeconds !== undefined && route.validation ? (
            <span className="ml-1 text-xs text-[var(--ink-2)]">
              {formatMovingTimeUncertainty(route.validation)}
            </span>
          ) : null}
        </Row>
        <Row term="Highest">
          {highestMetres === null ? "—" : formatElevation(highestMetres, unitSystem)}
        </Row>
        {climbLine === null ? null : <Row term="Climbs">{climbLine}</Row>}
      </dl>
      <ProportionChips
        name="Gradient"
        entries={bandEntries(bands, route.distanceMetres)}
        absence="No elevation data."
        highlight={highlight}
        onHighlightChange={onHighlightChange}
      />
      <ProportionChips
        name="Surface"
        entries={surfaceEntries(surface)}
        absence={surfaceAbsence}
        highlight={highlight}
        onHighlightChange={onHighlightChange}
      />
    </div>
  );
}
