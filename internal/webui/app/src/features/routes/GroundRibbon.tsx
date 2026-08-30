/**
 * A ribbon that names its own classes, once each, where they actually are.
 *
 * The ribbon says where the ground changes and the class list underneath says
 * what the classes are, and joining the two is a colour-matching exercise the
 * reader has to do every time. The sideways card in the route-panel spike
 * solved the same problem the other way up: leave the bar exactly
 * proportional, put the names beside it, and draw a line from each name to the
 * thing it names.
 *
 * Turned on its side that means one label per class rather than per stretch —
 * a route can cross asphalt eight times and does not need to be told eight
 * times — anchored under the longest run of that class, which is both the
 * easiest to point at and the one a reader is most likely to be asking about.
 *
 * Labels are laid out left to right and pushed apart where they would collide,
 * so the leader lines lean. A lean is the honest signal here: it says this
 * name has been moved from where its class is, and by how much.
 */

import type { SurfaceKind } from "../../api/types";
import type { Highlight } from "../../lib/highlight";
import type { Segment } from "../../lib/mix";
import { surfaceVariable } from "../../lib/mix";
import type { SurfaceSummary } from "../../lib/surface";
import { SURFACE_STYLES } from "../../lib/surface";
import { useElementWidth } from "../../lib/useElementWidth";
import { HighlightToggle } from "./HighlightToggle";
import { MixRibbon } from "./MixRibbon";

const LABEL_HEIGHT = 14;
const LEADER_HEIGHT = 9;
/** Room either side of a label, so two that survive the pass do not touch. */
const GAP = 10;
/** An over-estimate at eleven pixels, so the pass errs towards spreading out. */
const CHARACTER = 5.6;

interface Anchor {
  kind: SurfaceKind;
  label: string;
  description: string;
  colour: string;
  /** Where the class is, as a fraction of the route. */
  at: number;
  width: number;
}

/**
 * One anchor per class, under its longest run.
 *
 * The longest rather than the first: a route that touches gravel for two
 * hundred metres at kilometre three and then rides it for eleven kilometres
 * over the col should be pointing at the col.
 */
function anchorsFor(surface: SurfaceSummary | null): Anchor[] {
  if (surface === null || surface.totalMetres <= 0) {
    return [];
  }
  const longest = new Map<SurfaceKind, { start: number; end: number }>();
  for (const band of surface.bands) {
    const held = longest.get(band.kind);
    if (held === undefined || band.endMetres - band.startMetres > held.end - held.start) {
      longest.set(band.kind, { start: band.startMetres, end: band.endMetres });
    }
  }

  return [...longest.entries()]
    .map(([kind, band]) => ({
      kind,
      label: SURFACE_STYLES[kind].label,
      description: SURFACE_STYLES[kind].description,
      colour: surfaceVariable(kind),
      at: (band.start + band.end) / 2 / surface.totalMetres,
      width: SURFACE_STYLES[kind].label.length * CHARACTER + GAP,
    }))
    .sort((left, right) => left.at - right.at);
}

/** Where each label ends up, in pixels from the ribbon's left edge. */
function place(anchors: Anchor[], width: number): number[] {
  const centres: number[] = [];
  let rightmost = 0;
  for (const anchor of anchors) {
    const wanted = anchor.at * width - anchor.width / 2;
    const left = Math.max(wanted, rightmost);
    centres.push(left);
    rightmost = left + anchor.width;
  }

  // The forward pass can push the last label off the right edge. Walking back
  // from the edge gives every label the rightmost position it can still hold.
  if (rightmost > width) {
    let ceiling = width;
    for (let index = centres.length - 1; index >= 0; index--) {
      const anchor = anchors[index];
      const left = Math.max(Math.min(centres[index] ?? 0, ceiling - (anchor?.width ?? 0)), 0);
      centres[index] = left;
      ceiling = left;
    }
  }

  return centres;
}

export function GroundRibbon({
  segments,
  surface,
  labelled = true,
  highlight,
  onHighlightChange,
}: {
  segments: Segment[];
  surface: SurfaceSummary | null;
  /**
   * Whether the class names are shown.
   *
   * Only the names fold. The ribbon itself is positional — it says where the
   * gravel is, which nothing else in the sheet says — so folding it would
   * remove a reading rather than tidy one away. What can go is the key, which
   * a reader who knows the palette does not need on screen.
   */
  labelled?: boolean;
  highlight: Highlight | null;
  onHighlightChange: (next: Highlight | null) => void;
}) {
  const { ref, width } = useElementWidth<HTMLDivElement>();
  const anchors = anchorsFor(surface);
  const placed = place(anchors, width);

  return (
    <div ref={ref}>
      <MixRibbon segments={segments} className="h-3" highlight={highlight} />
      {!labelled ? null : (
        <div className="relative" style={{ height: LEADER_HEIGHT + LABEL_HEIGHT }}>
          <svg
            className="absolute inset-x-0 top-0"
            height={LEADER_HEIGHT}
            width="100%"
            aria-hidden="true"
          >
            {anchors.map((anchor, index) => {
              const from = anchor.at * width;
              const to = (placed[index] ?? 0) + anchor.width / 2;

              return (
                <path
                  key={anchor.kind}
                  d={`M${from.toFixed(1)} 0 L${to.toFixed(1)} ${LEADER_HEIGHT}`}
                  className="stroke-[var(--rule)]"
                  strokeWidth={1}
                  fill="none"
                />
              );
            })}
          </svg>
          {anchors.map((anchor, index) => (
            <HighlightToggle
              key={anchor.kind}
              highlight={{ type: "surface", kind: anchor.kind }}
              current={highlight}
              onChange={onHighlightChange}
              label={`${anchor.label}, ${anchor.description}`}
              title={anchor.description}
              className="absolute rounded text-[11px] leading-none text-[var(--ink-2)] hover:text-[var(--ink)] aria-pressed:font-semibold aria-pressed:text-[var(--ink)]"
              style={{ left: placed[index] ?? 0, top: LEADER_HEIGHT, height: LABEL_HEIGHT }}
            >
              {anchor.label}
            </HighlightToggle>
          ))}
        </div>
      )}
    </div>
  );
}
