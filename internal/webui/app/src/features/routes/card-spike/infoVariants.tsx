/**
 * Four takes on the route card in the Elera idiom: a dark icon square naming
 * the card, one weight of heading, figures as tiles or ledger rows rather than
 * a two-column list, tinted chips for the extremes, and a tinted callout for
 * the one sentence worth saying about the route.
 *
 * Storybook only. The mixes are held constant so what is being compared is how
 * the six figures are weighted and grouped.
 */

import {
  IconArrowBarDown,
  IconArrowBarUp,
  IconDots,
  IconMountain,
  IconRoute,
  IconRuler2,
  IconStopwatch,
  IconTrendingDown,
  IconTrendingUp,
  IconX,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { Highlight } from "../../../lib/highlight";
import { FIGURES, GRADIENT_MIX, SURFACE_MIX, TITLE } from "./data";
import { MixRow } from "./MixRow";

interface BodyProps {
  highlight: Highlight | null;
  onHighlightChange: (next: Highlight | null) => void;
}

/** The Elera card header: icon square, title, muted subtitle, actions at the end. */
function Header({ subtitle }: { subtitle?: string | undefined }) {
  return (
    <div className="flex items-center gap-3 px-5 pt-5">
      <span className="grid size-9 shrink-0 place-items-center rounded-md bg-[var(--ink)] text-[var(--panel)]">
        <IconRoute size={18} stroke={1.8} aria-hidden="true" />
      </span>
      <div className="min-w-0 flex-1">
        <h2 className="truncate font-semibold text-base leading-tight">{TITLE}</h2>
        {subtitle ? <p className="text-[var(--ink-2)] text-xs">{subtitle}</p> : null}
      </div>
      <button type="button" className="rounded-lg p-1.5 text-[var(--ink-2)]">
        <IconDots size={16} stroke={2} aria-hidden="true" />
      </button>
      <button type="button" className="rounded-lg p-1.5 text-[var(--ink-2)]">
        <IconX size={16} stroke={2} aria-hidden="true" />
      </button>
    </div>
  );
}

function Shell({ subtitle, children }: { subtitle?: string; children: ReactNode }) {
  return (
    <div className="w-[24rem] overflow-hidden rounded-2xl bg-[var(--panel)] shadow-[var(--shadow)]">
      <Header subtitle={subtitle} />
      <div className="grid gap-4 px-5 pt-4 pb-5">{children}</div>
    </div>
  );
}

function Mixes({ highlight, onHighlightChange }: BodyProps) {
  return (
    <div className="grid gap-0.5">
      <MixRow
        name="Gradient"
        classesLabel="Gradient bands"
        entries={GRADIENT_MIX}
        absence="No elevation data."
        tagSide="above"
        highlight={highlight}
        onHighlightChange={onHighlightChange}
      />
      <MixRow
        name="Surface"
        classesLabel="Surface classes"
        entries={SURFACE_MIX}
        absence="No surface data."
        tagSide="below"
        highlight={highlight}
        onHighlightChange={onHighlightChange}
      />
    </div>
  );
}

/** A soft tinted chip in one of the page's tones. */
function Chip({
  tone,
  children,
}: {
  tone: "good" | "hold" | "alert" | "info";
  children: ReactNode;
}) {
  const colour = tone === "info" ? "var(--accent)" : `var(--${tone})`;
  return (
    <span
      className="inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 font-medium text-[11px] tabular-nums"
      style={{ color: colour, background: `color-mix(in oklab, ${colour} 12%, transparent)` }}
    >
      {children}
    </span>
  );
}

const Grades = () => (
  <span className="inline-flex gap-1">
    <Chip tone="alert">
      <IconTrendingUp size={12} stroke={2} aria-hidden="true" />
      {FIGURES.steepestClimbing}
    </Chip>
    <Chip tone="hold">
      <IconTrendingDown size={12} stroke={2} aria-hidden="true" />
      {FIGURES.steepestDescent}
    </Chip>
  </span>
);

/* ------------------------------------------------------------------------ */

function Tile({ label, value, foot }: { label: string; value: ReactNode; foot?: ReactNode }) {
  return (
    <div className="grid gap-0.5 rounded-xl bg-[var(--base)] px-3 py-2.5">
      <span className="text-[11px] text-[var(--ink-2)]">{label}</span>
      <span className="font-semibold text-base leading-tight tabular-nums">{value}</span>
      {foot ? <span className="text-[11px] text-[var(--ink-2)]">{foot}</span> : null}
    </div>
  );
}

/** A · Tiles: six figures as a grid of quiet tiles, the way the dashboard strip does its KPIs. */
export function TilesCard(props: BodyProps) {
  return (
    <Shell>
      <div className="grid grid-cols-2 gap-2">
        <Tile label="Distance" value={FIGURES.distance} />
        <Tile label="Moving time" value={FIGURES.movingTime} foot={FIGURES.uncertainty} />
        <Tile
          label="Ascent"
          value={FIGURES.ascent}
          foot={
            <span className="inline-flex items-center gap-0.5">
              <IconArrowBarDown size={11} stroke={2} aria-hidden="true" />
              1,820 m
            </span>
          }
        />
        <Tile label="Elevation" value={FIGURES.elevation} />
        <Tile label="Avg climbing" value={FIGURES.averageClimbing} />
        <Tile label="Max grade" value={<Grades />} />
      </div>
      <Mixes {...props} />
    </Shell>
  );
}

/* ------------------------------------------------------------------------ */

function Row({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3 border-[var(--rule)] border-b py-2 last:border-b-0">
      <dt className="text-[13px] text-[var(--ink-2)]">{label}</dt>
      <dd className="font-medium text-sm tabular-nums">{children}</dd>
    </div>
  );
}

/** B · Ledger: two hero figures decide the ride; the rest are hairline rows beneath. */
export function LedgerCard(props: BodyProps) {
  return (
    <Shell subtitle="Loop · Kaiserstuhl">
      <div className="grid grid-cols-2 gap-3">
        <div>
          <div className="text-[11px] text-[var(--ink-2)]">Distance</div>
          <div className="font-semibold text-2xl leading-tight tabular-nums">
            {FIGURES.distance}
          </div>
        </div>
        <div>
          <div className="text-[11px] text-[var(--ink-2)]">Moving time</div>
          <div className="font-semibold text-2xl leading-tight tabular-nums">
            {FIGURES.movingTime}
          </div>
          <div className="text-[11px] text-[var(--ink-2)]">{FIGURES.uncertainty}</div>
        </div>
      </div>
      <dl>
        <Row label="Ascent / descent">
          <span className="inline-flex items-center gap-1">
            <IconArrowBarUp size={12} stroke={2} aria-hidden="true" />
            {FIGURES.ascent}
            <span className="text-[var(--ink-2)]">/</span>
            <IconArrowBarDown size={12} stroke={2} aria-hidden="true" />
            1,820 m
          </span>
        </Row>
        <Row label="Elevation">{FIGURES.elevation}</Row>
        <Row label="Avg climbing">{FIGURES.averageClimbing}</Row>
        <Row label="Max grade">
          <Grades />
        </Row>
      </dl>
      <Mixes {...props} />
    </Shell>
  );
}

/* ------------------------------------------------------------------------ */

function Cell({ icon, label, value }: { icon: ReactNode; label: string; value: ReactNode }) {
  return (
    <div className="flex items-center gap-2.5">
      <span className="grid size-8 shrink-0 place-items-center rounded-md bg-[var(--ink)] text-[var(--panel)]">
        {icon}
      </span>
      <div className="min-w-0">
        <div className="text-[11px] text-[var(--ink-2)]">{label}</div>
        <div className="font-semibold text-base leading-tight tabular-nums">{value}</div>
      </div>
    </div>
  );
}

/** C · Strip: the dashboard's KPI strip, icon squares and all, then the rest as one quiet line. */
export function StripCard(props: BodyProps) {
  return (
    <Shell>
      <div className="grid grid-cols-2 gap-x-4 gap-y-3">
        <Cell
          icon={<IconRuler2 size={16} stroke={1.8} aria-hidden="true" />}
          label="Distance"
          value={FIGURES.distance}
        />
        <Cell
          icon={<IconStopwatch size={16} stroke={1.8} aria-hidden="true" />}
          label="Moving time"
          value={FIGURES.movingTime}
        />
        <Cell
          icon={<IconArrowBarUp size={16} stroke={1.8} aria-hidden="true" />}
          label="Ascent"
          value={FIGURES.ascent}
        />
        <Cell
          icon={<IconMountain size={16} stroke={1.8} aria-hidden="true" />}
          label="Elevation"
          value={FIGURES.elevation}
        />
      </div>
      <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[12px] text-[var(--ink-2)]">
        <span>
          Avg climbing <b className="font-medium text-[var(--ink)]">{FIGURES.averageClimbing}</b>
        </span>
        <span aria-hidden="true">·</span>
        <span className="inline-flex items-center gap-1">
          Max grade <Grades />
        </span>
        <span aria-hidden="true">·</span>
        <span>{FIGURES.uncertainty}</span>
      </p>
      <Mixes {...props} />
    </Shell>
  );
}

/* ------------------------------------------------------------------------ */

/** D · Callout: the ledger, plus the one sentence the figures add up to, in a tinted bar. */
export function CalloutCard(props: BodyProps) {
  return (
    <Shell subtitle={`${FIGURES.distance} · ${FIGURES.movingTime} · ${FIGURES.ascent}`}>
      <dl>
        <Row label="Elevation">{FIGURES.elevation}</Row>
        <Row label="Ascent / descent">
          {FIGURES.ascent} <span className="text-[var(--ink-2)]">/</span> 1,820 m
        </Row>
        <Row label="Avg climbing">{FIGURES.averageClimbing}</Row>
        <Row label="Max grade">
          <Grades />
        </Row>
      </dl>
      <Mixes {...props} />
      <div
        className="flex gap-3 rounded-xl p-3 text-[12px]"
        style={{
          color: "var(--hold)",
          background: "color-mix(in oklab, var(--hold) 10%, transparent)",
        }}
      >
        <span
          className="grid size-7 shrink-0 place-items-center rounded-sm text-[var(--panel)]"
          style={{ background: "var(--hold)" }}
        >
          <IconMountain size={14} stroke={2} aria-hidden="true" />
        </span>
        <p>
          <b className="font-semibold">Over half the route is 9% or steeper.</b> 13.7 km sits above
          12%, most of it on gravel and ground — plan the moving time as a floor.
        </p>
      </div>
    </Shell>
  );
}
