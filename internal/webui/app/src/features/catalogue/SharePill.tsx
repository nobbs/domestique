/**
 * One class of a route's composition, as a badge that is also its own bar.
 *
 * The fill runs from the left to exactly the share the label states, so the
 * badge reads either way round: at a glance as a proportion, or properly as a
 * figure. A row of them is the division of the whole route, which is why there
 * is no strip above them stating the same division a second time.
 *
 * The colour goes under the label rather than behind it. Both palettes this
 * draws from run from pale to dark — the gradient bands are grey, green,
 * yellow, orange, red — and the darkest of them leave the page's own ink at
 * about three to one against an eleven-pixel label, under the four and a half
 * it wants. Keeping the two apart is what lets both palettes be used at full
 * strength with nothing muted.
 */
export function SharePill({
  colour,
  label,
  share,
}: {
  /** A CSS colour, normally one of the palette's custom properties. */
  colour: string;
  label: string;
  /** Of the whole route, from 0 to 1. */
  share: number;
}) {
  const percent = Math.round(share * 100);

  return (
    <span className="relative inline-block rounded-md bg-[var(--base)] px-2 pt-0.5 pb-2 text-[11px] text-[var(--ink)] tabular-nums">
      {label} {percent}%
      <span className="absolute inset-x-1.5 bottom-1 block h-1 overflow-hidden rounded-full bg-[color-mix(in_srgb,var(--ink)_14%,transparent)]">
        <span
          className="block h-full rounded-full"
          style={{ width: `${percent}%`, background: colour }}
        />
      </span>
    </span>
  );
}
