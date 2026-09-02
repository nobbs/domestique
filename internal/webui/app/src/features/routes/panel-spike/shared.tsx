/**
 * What all four cards agree on, so the comparison is about layout.
 *
 * Each alternative draws the two mixes its own way — that is the point of the
 * spike — but none of them may quietly invent its own palette, its own class
 * names, or its own idea of what pressing a class does. Those live here once,
 * and a card that looked better because it had renamed "Unsurveyed" to
 * "Unknown" would be winning an argument nobody was having.
 */

import type { ReactNode } from "react";
import type { Route, SurfaceKind } from "../../../api/types";
import type { Climb } from "../../../lib/climbs";
import { formatDistance, formatGradient } from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import { sameHighlight } from "../../../lib/highlight";
import type { BandShare } from "../../../lib/profile";
import { GRADIENT_BANDS } from "../../../lib/profile";
import { providerLabel } from "../../../lib/provider";
import type { SurfaceSummary } from "../../../lib/surface";
import { SURFACE_STYLES } from "../../../lib/surface";
import type { UnitSystem } from "../../../lib/units";

/**
 * The custom property each class is painted from.
 *
 * The properties rather than `bandColour`/`surfaceColour`, which take the
 * cartography's own darkness because they paint marks *on the map*. Nothing
 * here is on the map, so these follow the page's theme like every other colour
 * in the panel.
 */
export function bandVariable(band: number): string {
  return `var(--grade-${band})`;
}

const SURFACE_VARIABLE: Record<SurfaceKind, string> = {
  asphalt: "--surface-asphalt",
  paving: "--surface-paving",
  compacted: "--surface-compacted",
  gravel: "--surface-gravel",
  ground: "--surface-ground",
  unknown: "--surface-unsurveyed",
};

export function surfaceVariable(kind: SurfaceKind): string {
  return `var(${SURFACE_VARIABLE[kind]})`;
}

export function bandLabel(band: number): string {
  return GRADIENT_BANDS[band]?.label ?? "";
}

export function bandDescription(band: number): string {
  return GRADIENT_BANDS[band]?.description ?? "";
}

export function surfaceLabel(kind: SurfaceKind): string {
  return SURFACE_STYLES[kind].label;
}

/** `0.084` as `8%`, and anything that would round to nothing as `<1%`. */
export function formatShare(share: number): string {
  const percent = share * 100;

  return percent < 0.5 && percent > 0 ? "<1%" : `${Math.round(percent)}%`;
}

/**
 * What the card says above its content.
 *
 * Not the route's name: the pill directly above is carrying it, and a card
 * that repeated it would spend its first row telling a reader something they
 * are already looking at. What is left is where the route came from and when
 * it was read, which the pill has no room for.
 */
export function CardHeading({ route, subtitle }: { route: Route; subtitle: string }) {
  return (
    <p className="text-xs text-[var(--ink-2)]">
      <span className="font-semibold tracking-[0.06em] uppercase">
        {providerLabel(route.provider)}
      </span>
      {subtitle === "" ? null : ` · ${subtitle}`}
    </p>
  );
}

export interface CardProps {
  route: Route;
  subtitle: string;
  /** The whole route's, or the stretch a map selection has picked out. */
  movingSeconds: number | undefined;
  highestMetres: number | null;
  /** How much of the route each steepness band covers, gentlest first. */
  bands: BandShare[];
  /** The same steepness in the order it is ridden, for a card that draws where. */
  runs: BandShare[];
  surface: SurfaceSummary | null;
  surfaceAbsence: string;
  climbs: Climb[];
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
  unitSystem: UnitSystem;
}

/**
 * One class, pressable.
 *
 * Every alternative makes something pressable — a chip, a ribbon segment, a
 * word in a sentence, a slice of a column — and all four have to agree that
 * pressing the pressed one gives the whole route back. Only the appearance is
 * the card's business, so this carries none.
 */
export function HighlightToggle({
  highlight,
  current,
  onChange,
  label,
  className,
  style,
  title,
  children,
}: {
  highlight: Highlight;
  current: Highlight | null;
  onChange: (next: Highlight | null) => void;
  /** Spoken instead of the contents, which are usually an abbreviation. */
  label: string;
  className?: string;
  style?: React.CSSProperties;
  title?: string;
  children: ReactNode;
}) {
  const pressed = sameHighlight(current, highlight);

  return (
    <button
      type="button"
      aria-pressed={pressed}
      aria-label={label}
      title={title}
      onClick={() => onChange(pressed ? null : highlight)}
      className={`focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] ${className ?? ""}`}
      style={style}
    >
      {children}
    </button>
  );
}

/**
 * The route's climbs in one line: how many, and the one that decides the day.
 *
 * The list itself is not here — it belongs to the wide panel, where there is
 * room to put every climb in ride order. What a reader choosing a route needs
 * from it is the count and the worst of them, and both fit on a line.
 */
export function climbSentence(climbs: Climb[], unitSystem: UnitSystem): string | null {
  const biggest = climbs.reduce<Climb | null>(
    (worst, climb) => (worst === null || climb.ascentMetres > worst.ascentMetres ? climb : worst),
    null,
  );
  if (biggest === null) {
    return null;
  }

  return `${climbs.length} ${climbs.length === 1 ? "climb" : "climbs"} · biggest ${formatDistance(biggest.distanceMetres, unitSystem)} at ${formatGradient(biggest.averageGradePercent)}`;
}

/** Every class present in the two mixes, as the rows a card draws. */
export interface MixEntry {
  highlight: Highlight;
  label: string;
  description: string;
  share: number;
  /** The same quantity as ground rather than as a proportion. */
  metres: number;
  colour: string;
}

/**
 * The steepness bands as rows.
 *
 * The route's own length is needed because a band knows only its share of the
 * route. It is the figure the panel prints everywhere else, so a card that
 * multiplied by the geometry's measured length instead could report a total
 * that does not add up to the distance beside it.
 */
export function bandEntries(bands: BandShare[], totalMetres: number): MixEntry[] {
  return bands.map((entry) => ({
    highlight: { type: "band", band: entry.band },
    label: bandLabel(entry.band),
    description: bandDescription(entry.band),
    share: entry.share,
    metres: entry.share * totalMetres,
    colour: bandVariable(entry.band),
  }));
}

export function surfaceEntries(surface: SurfaceSummary | null): MixEntry[] {
  return (surface?.shares ?? []).map((entry) => ({
    highlight: { type: "surface", kind: entry.kind },
    label: surfaceLabel(entry.kind),
    description: SURFACE_STYLES[entry.kind].description,
    share: entry.share,
    // Measured by the classifier rather than derived from the share, which is
    // the same number one rounding earlier.
    metres: entry.metres,
    colour: surfaceVariable(entry.kind),
  }));
}

/** One class's stretch of the route, as the ribbons draw it. */
export interface Segment {
  key: string;
  colour: string;
  /** Of the whole route, from 0 to 1. */
  share: number;
  highlight: Highlight;
}

/**
 * A mix drawn in the order it is ridden.
 *
 * Substrate rather than one card's idea: two alternatives draw the route this
 * way and they must agree about what the picture means, or the comparison
 * between them becomes a comparison of two different claims. What each does
 * around it — an axis and bracketed cols, or nothing at all — is theirs.
 *
 * Hidden from assistive technology, like the proportion bar it stands in for.
 * Every class in it is named with its share in the prose or the list beside
 * it, and a picture of an order cannot be read out as one anyway.
 */
export function Ribbon({
  segments,
  className,
  highlight,
}: {
  segments: Segment[];
  /** The ribbon's own height, which is the only thing that varies between cards. */
  className: string;
  highlight: Highlight | null;
}) {
  return (
    <div className={`flex w-full overflow-hidden rounded-[3px] ${className}`} aria-hidden="true">
      {segments.map((segment) => (
        <div
          key={segment.key}
          style={{
            flexGrow: segment.share,
            flexBasis: 0,
            background: segment.colour,
            // Picking a class fades everything that is not it, which is the
            // answer the map gives to the same press. A ribbon is a map of the
            // route by distance, so it fades the same way.
            opacity: highlight === null || sameHighlight(highlight, segment.highlight) ? 1 : 0.16,
          }}
        />
      ))}
    </div>
  );
}

/** The steepness runs and the ground bands as ribbon segments. */
export function gradientSegments(runs: BandShare[]): Segment[] {
  return runs.map((run, index) => ({
    key: `${index}`,
    colour: bandVariable(run.band),
    share: run.share,
    highlight: { type: "band", band: run.band },
  }));
}

export function groundSegments(surface: SurfaceSummary | null): Segment[] {
  if (surface === null || surface.totalMetres <= 0) {
    return [];
  }

  return surface.bands.map((band, index) => ({
    key: `${index}`,
    colour: surfaceVariable(band.kind),
    share: (band.endMetres - band.startMetres) / surface.totalMetres,
    highlight: { type: "surface", kind: band.kind },
  }));
}
