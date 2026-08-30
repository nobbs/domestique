/**
 * Five ways the route card could divide, label and weight what it holds.
 *
 * The card is right about *what* it says and unconvincing about *how*. Three
 * things carry that: the rules between its sections, the lines that fold those
 * sections away, and the treatment of the figures themselves. Each variant
 * below takes a different position on all three rather than nudging one, so
 * comparing them is a choice between ideas rather than between margins.
 *
 * The pill header is held constant on purpose. It is the one part nobody has
 * questioned, and varying it would make every comparison below noisier.
 */

import {
  IconArrowLeft,
  IconChevronDown,
  IconChevronRight,
  IconDots,
  IconTrendingDown,
  IconTrendingUp,
  IconX,
} from "@tabler/icons-react";
import { type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { MixColumn } from "../../../components/route/MixColumn";
import type { Highlight } from "../../../lib/highlight";
import {
  CLIMB_SUMMARY,
  CLIMBS,
  FIGURES,
  GRADIENT_MIX,
  MIX_SUMMARY,
  SURFACE_MIX,
  TITLE,
} from "./data";
import { MixRow } from "./MixRow";

/** The parts every variant shares, so the differences below are the variants'. */
interface BodyProps {
  highlight: Highlight | null;
  onHighlightChange: (next: Highlight | null) => void;
}

function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="w-[23rem] overflow-hidden rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] ring-1 ring-black/5">
      <div className="flex items-center gap-1 p-1.5">
        <button
          type="button"
          className="flex min-w-0 items-center gap-2 rounded-lg px-2 py-1 text-left hover:bg-[var(--base)]"
        >
          <IconChevronDown size={16} stroke={2} aria-hidden="true" />
          <span className="truncate font-semibold">{TITLE}</span>
        </button>
        <button type="button" className="ml-auto rounded-lg p-1.5 text-[var(--ink-2)]">
          <IconDots size={16} stroke={2} aria-hidden="true" />
        </button>
        <button type="button" className="rounded-lg p-1.5 text-[var(--ink-2)]">
          <IconX size={16} stroke={2} aria-hidden="true" />
        </button>
      </div>
      {children}
    </div>
  );
}

function Mixes({ highlight, onHighlightChange }: BodyProps) {
  return (
    <div className="flex items-start gap-3">
      <MixColumn
        name="Gradient"
        classesLabel="Gradient bands"
        entries={GRADIENT_MIX}
        absence="No elevation data."
        highlight={highlight}
        onHighlightChange={onHighlightChange}
        unitSystem="metric"
      />
      <MixColumn
        name="Surface"
        classesLabel="Surface classes"
        entries={SURFACE_MIX}
        absence="Surface not classified yet."
        highlight={highlight}
        onHighlightChange={onHighlightChange}
        unitSystem="metric"
      />
    </div>
  );
}

function ClimbRows() {
  return (
    <div className="text-[11px] tabular-nums">
      <div
        className="grid gap-2 px-1.5 pb-1 text-[10px] text-[var(--ink-2)]"
        style={{ gridTemplateColumns: "0.75rem 3.5rem 2.5rem 2.5rem 3.5rem minmax(0,1fr)" }}
      >
        <span />
        <span className="text-right">Length</span>
        <span className="text-right">Avg</span>
        <span className="text-right">Max</span>
        <span className="text-right">Ascent</span>
        <span className="text-right">Starts</span>
      </div>
      {CLIMBS.map((climb) => (
        <div
          key={climb.ordinal}
          className="grid gap-2 rounded px-1.5 py-1 hover:bg-[var(--base)]"
          style={{ gridTemplateColumns: "0.75rem 3.5rem 2.5rem 2.5rem 3.5rem minmax(0,1fr)" }}
        >
          <span className="text-[var(--ink-2)]">{climb.ordinal}</span>
          <span className="text-right">{climb.length}</span>
          <span className="text-right">{climb.average}</span>
          <span className="text-right">{climb.steepest}</span>
          <span className="text-right">{climb.ascent}</span>
          <span className="text-right text-[var(--ink-2)]">{climb.starts}</span>
        </div>
      ))}
    </div>
  );
}

/* ------------------------------------------------------------------ A: Air */

/**
 * No rules at all. Space does the dividing, and each section names itself in
 * the same small caps the mixes already use for their columns — so the card
 * has one vocabulary for "a new thing starts here" instead of two.
 *
 * The bet: the rules were never carrying meaning, only reassurance, and three
 * of them in a card this short read as a form rather than as a summary.
 */
function AirSection({
  name,
  summary,
  children,
}: {
  name: string;
  summary: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);

  return (
    <section>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="group flex w-full items-baseline gap-2 py-1 text-left"
      >
        <span className="text-[10px] font-semibold tracking-[0.08em] text-[var(--ink-2)] uppercase">
          {name}
        </span>
        <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--ink-2)] group-hover:text-[var(--ink)]">
          {open ? "" : summary}
        </span>
        <IconChevronRight
          size={12}
          stroke={2}
          aria-hidden="true"
          className={
            open ? "shrink-0 rotate-90 text-[var(--ink-2)]" : "shrink-0 text-[var(--ink-2)]"
          }
        />
      </button>
      {open ? <div className="pt-1 pb-2">{children}</div> : null}
    </section>
  );
}

export function AirCard({ withClimbs = true, ...props }: BodyProps & { withClimbs?: boolean }) {
  return (
    <Shell>
      <div className="grid gap-4 px-3 pt-1 pb-3">
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5">
          <AirFigure term="Distance" value={FIGURES.distance} />
          <AirFigure term="Ascent" value={FIGURES.ascent} />
          <AirFigure term="Elevation" value={FIGURES.elevation} />
          <AirFigure term="Avg climbing" value={FIGURES.averageClimbing} />
          <AirFigure term="Max climb" value={FIGURES.steepestClimbing} icon="up" />
          <AirFigure term="Max descent" value={FIGURES.steepestDescent} icon="down" />
          <AirFigure term="Moving time" value={FIGURES.movingTime} span />
        </dl>
        <div className="grid gap-1">
          <AirSection name="Mix" summary={MIX_SUMMARY}>
            <Mixes {...props} />
          </AirSection>
          {withClimbs ? (
            <AirSection name="Climbs" summary={CLIMB_SUMMARY}>
              <ClimbRows />
            </AirSection>
          ) : null}
        </div>
      </div>
    </Shell>
  );
}

function AirFigure({
  term,
  value,
  icon,
  span,
}: {
  term: string;
  value: string;
  icon?: "up" | "down";
  span?: boolean;
}) {
  return (
    <div className={`flex items-baseline justify-between gap-2 ${span ? "col-span-2" : ""}`}>
      <dt className="flex min-w-0 items-center gap-1 truncate text-[11px] text-[var(--ink-2)]">
        {icon === "up" ? <IconTrendingUp size={12} stroke={2} aria-hidden="true" /> : null}
        {icon === "down" ? <IconTrendingDown size={12} stroke={2} aria-hidden="true" /> : null}
        {term}
      </dt>
      <dd className="shrink-0 text-sm leading-tight tabular-nums">{value}</dd>
    </div>
  );
}

/* ---------------------------------------------------------------- B: Bands */

/**
 * The sections sit on their own ground instead of under their own rule. The
 * figures keep the card's colour and the two folds take a tint, so the card
 * reads as a headline with two drawers rather than as three equal thirds.
 *
 * The bet: what the rules were trying to say is "these are different kinds of
 * thing", and a change of ground says that louder than a line.
 */
function BandSection({
  name,
  summary,
  children,
}: {
  name: string;
  summary: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);

  return (
    <section className="bg-[var(--base)]">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs hover:bg-[color-mix(in_srgb,var(--ink)_5%,transparent)]"
      >
        <IconChevronRight
          size={12}
          stroke={2}
          aria-hidden="true"
          className={open ? "shrink-0 rotate-90" : "shrink-0"}
        />
        <span className="font-semibold">{name}</span>
        <span className="min-w-0 flex-1 truncate text-right text-[var(--ink-2)]">
          {open ? "" : summary}
        </span>
      </button>
      {open ? <div className="px-3 pb-3">{children}</div> : null}
    </section>
  );
}

export function BandCard(props: BodyProps) {
  return (
    <Shell>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 px-3 pt-1 pb-3">
        <AirFigure term="Distance" value={FIGURES.distance} />
        <AirFigure term="Ascent" value={FIGURES.ascent} />
        <AirFigure term="Elevation" value={FIGURES.elevation} />
        <AirFigure term="Avg climbing" value={FIGURES.averageClimbing} />
        <AirFigure term="Max climb" value={FIGURES.steepestClimbing} icon="up" />
        <AirFigure term="Max descent" value={FIGURES.steepestDescent} icon="down" />
        <AirFigure term="Moving time" value={FIGURES.movingTime} span />
      </dl>
      <div className="grid gap-px">
        <BandSection name="Mix" summary={MIX_SUMMARY}>
          <Mixes {...props} />
        </BandSection>
        <BandSection name="Climbs" summary={CLIMB_SUMMARY}>
          <ClimbRows />
        </BandSection>
      </div>
    </Shell>
  );
}

/* --------------------------------------------------------------- C: Ledger */

/**
 * Leans into the card being a table of numbers. The figures get dotted leaders
 * so the eye crosses from name to number without the gap between them reading
 * as a mistake, and the rules stay — but inset to the content and only between
 * sections, so they divide rather than box.
 *
 * The bet: the card's discomfort is that its figures float in the middle
 * distance, neither a list nor a table, and committing to the table fixes it.
 */
function LedgerFigure({
  term,
  value,
  icon,
}: {
  term: string;
  value: string;
  icon?: "up" | "down";
}) {
  return (
    <div className="flex min-w-0 items-baseline gap-1 text-[var(--ink-2)]">
      <dt className="flex min-w-0 shrink items-center gap-1 truncate text-[11px]">
        {icon === "up" ? <IconTrendingUp size={12} stroke={2} aria-hidden="true" /> : null}
        {icon === "down" ? <IconTrendingDown size={12} stroke={2} aria-hidden="true" /> : null}
        {term}
      </dt>
      <span
        aria-hidden="true"
        className="min-w-3 flex-1 translate-y-[-3px] border-b border-dotted border-[var(--rule)]"
      />
      <dd className="shrink-0 text-sm leading-tight tabular-nums text-[var(--ink)]">{value}</dd>
    </div>
  );
}

function LedgerSection({
  name,
  summary,
  children,
}: {
  name: string;
  summary: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);

  return (
    <section className="border-t border-[var(--rule)] pt-2">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-baseline gap-2 text-left text-xs"
      >
        <span className="font-semibold">{name}</span>
        <span className="min-w-0 flex-1 truncate text-right text-[11px] text-[var(--ink-2)]">
          {open ? "" : summary}
        </span>
        <IconChevronRight
          size={12}
          stroke={2}
          aria-hidden="true"
          className={
            open ? "shrink-0 rotate-90 text-[var(--ink-2)]" : "shrink-0 text-[var(--ink-2)]"
          }
        />
      </button>
      {open ? <div className="pt-2">{children}</div> : null}
    </section>
  );
}

export function LedgerCard(props: BodyProps) {
  return (
    <Shell>
      <div className="grid gap-2 px-3 pt-1 pb-3">
        <dl className="grid grid-cols-2 gap-x-5 gap-y-1">
          <LedgerFigure term="Distance" value={FIGURES.distance} />
          <LedgerFigure term="Ascent" value={FIGURES.ascent} />
          <LedgerFigure term="Elevation" value={FIGURES.elevation} />
          <LedgerFigure term="Avg climbing" value={FIGURES.averageClimbing} />
          <LedgerFigure term="Max climb" value={FIGURES.steepestClimbing} icon="up" />
          <LedgerFigure term="Max descent" value={FIGURES.steepestDescent} icon="down" />
          <div className="col-span-2">
            <LedgerFigure term="Moving time" value={FIGURES.movingTime} />
          </div>
        </dl>
        <LedgerSection name="Mix" summary={MIX_SUMMARY}>
          <Mixes {...props} />
        </LedgerSection>
        <LedgerSection name="Climbs" summary={CLIMB_SUMMARY}>
          <ClimbRows />
        </LedgerSection>
      </div>
    </Shell>
  );
}

/* ------------------------------------------------------------ D: Hierarchy */

/**
 * Stops treating seven figures as seven equals. Distance and ascent decide
 * whether the ride is on; the other five are checked once and then not read
 * again, so they go to one quiet run underneath instead of five labelled rows.
 *
 * The bet: the card looks busy because everything on it is shouting at the
 * same volume, and the fix is editorial rather than decorative.
 */
export function HierarchyCard(props: BodyProps) {
  return (
    <Shell>
      <div className="grid gap-3 px-3 pt-1 pb-3">
        <div className="flex items-baseline gap-5">
          <div>
            <div className="text-[10px] tracking-[0.08em] text-[var(--ink-2)] uppercase">
              Distance
            </div>
            <div className="text-xl leading-tight font-semibold tabular-nums">
              {FIGURES.distance}
            </div>
          </div>
          <div>
            <div className="text-[10px] tracking-[0.08em] text-[var(--ink-2)] uppercase">
              Ascent
            </div>
            <div className="text-xl leading-tight font-semibold tabular-nums">{FIGURES.ascent}</div>
          </div>
          <div className="ml-auto text-right">
            <div className="text-[10px] tracking-[0.08em] text-[var(--ink-2)] uppercase">
              Moving
            </div>
            <div className="text-xl leading-tight font-semibold tabular-nums">
              {FIGURES.movingTime}
            </div>
          </div>
        </div>
        {/*
         * The five that are checked once: read as a sentence, in the order a
         * rider would ask them, rather than as a table nobody reads twice.
         */}
        <p className="text-[11px] leading-relaxed text-[var(--ink-2)] tabular-nums">
          Climbs at{" "}
          <span className="font-semibold text-[var(--ink)]">{FIGURES.averageClimbing}</span> on
          average, steepest{" "}
          <span className="font-semibold text-[var(--ink)]">{FIGURES.steepestClimbing}</span> up and{" "}
          <span className="font-semibold text-[var(--ink)]">{FIGURES.steepestDescent}</span> down ·
          between <span className="font-semibold text-[var(--ink)]">{FIGURES.elevation}</span>
        </p>
        <div className="grid gap-1.5">
          <AirSection name="Mix" summary={MIX_SUMMARY}>
            <Mixes {...props} />
          </AirSection>
          <AirSection name="Climbs" summary={CLIMB_SUMMARY}>
            <ClimbRows />
          </AirSection>
        </div>
      </div>
    </Shell>
  );
}

/* ----------------------------------------------------------------- E: Rail */

/**
 * Divides down rather than across. Each folded section hangs off a short
 * vertical rule at the card's left margin, so the eye follows one edge from
 * the top of the card to the bottom instead of stepping over three horizontal
 * bars — and an open section is visibly *inside* something.
 *
 * The bet: horizontal rules cut the card into pieces, and this is the same
 * division drawn as structure rather than as interruption.
 */
function RailSection({
  name,
  summary,
  children,
}: {
  name: string;
  summary: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);

  return (
    <section className="border-l-2 border-[var(--rule)] pl-2.5 hover:border-[var(--ink-2)]">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-baseline gap-2 text-left text-xs"
      >
        <span className="font-semibold">{name}</span>
        <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--ink-2)]">
          {open ? "" : summary}
        </span>
        <IconChevronRight
          size={12}
          stroke={2}
          aria-hidden="true"
          className={
            open ? "shrink-0 rotate-90 text-[var(--ink-2)]" : "shrink-0 text-[var(--ink-2)]"
          }
        />
      </button>
      {open ? <div className="pt-2 pb-1">{children}</div> : null}
    </section>
  );
}

export function RailCard(props: BodyProps) {
  return (
    <Shell>
      <div className="grid gap-3 px-3 pt-1 pb-3">
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1 border-l-2 border-transparent pl-2.5">
          <AirFigure term="Distance" value={FIGURES.distance} />
          <AirFigure term="Ascent" value={FIGURES.ascent} />
          <AirFigure term="Elevation" value={FIGURES.elevation} />
          <AirFigure term="Avg climbing" value={FIGURES.averageClimbing} />
          <AirFigure term="Max climb" value={FIGURES.steepestClimbing} icon="up" />
          <AirFigure term="Max descent" value={FIGURES.steepestDescent} icon="down" />
          <AirFigure term="Moving time" value={FIGURES.movingTime} span />
        </dl>
        <RailSection name="Mix" summary={MIX_SUMMARY}>
          <Mixes {...props} />
        </RailSection>
        <RailSection name="Climbs" summary={CLIMB_SUMMARY}>
          <ClimbRows />
        </RailSection>
      </div>
    </Shell>
  );
}

/* ---------------------------------------------------------------- F: Slide */

/**
 * A's typography, with its folds replaced by a slide.
 *
 * Opening a section stopped meaning "grow the card" and started meaning "go
 * there": the figures leave to the left and the section arrives in their
 * place, with the way back where a reader already looks for it. The card
 * keeps one job on screen at a time.
 *
 * Two things fold-out could not do. The card no longer changes how much map it
 * covers depending on what a reader opened — the camera frames a route around
 * a panel whose height is now nearly constant. And a section gets the whole
 * width and the whole height for as long as it is the thing being read,
 * instead of the room left over beneath six figures.
 *
 * The height still animates, because the three views genuinely differ in
 * length and snapping between them reads as a glitch rather than as a move.
 */
const VIEWS = ["figures", "mix", "climbs"] as const;
type View = (typeof VIEWS)[number];

const VIEW_NAMES: Record<View, string> = {
  figures: "Route",
  mix: "Gradient and surface",
  climbs: "Climbs",
};

function SlideRow({
  name,
  summary,
  onOpen,
}: {
  name: string;
  summary: string;
  onOpen: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="group flex w-full items-baseline gap-2 rounded py-1 text-left hover:bg-[var(--base)]"
    >
      <span className="text-[10px] font-semibold tracking-[0.08em] text-[var(--ink-2)] uppercase">
        {name}
      </span>
      <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--ink-2)] group-hover:text-[var(--ink)]">
        {summary}
      </span>
      <IconChevronRight
        size={12}
        stroke={2}
        aria-hidden="true"
        className="shrink-0 text-[var(--ink-2)] transition-transform group-hover:translate-x-0.5"
      />
    </button>
  );
}

/** The way back, in the place the section's own name would otherwise sit. */
function SlideHeader({ name, onBack }: { name: string; onBack: () => void }) {
  return (
    <button
      type="button"
      onClick={onBack}
      className="group -ml-1 flex w-full items-center gap-1.5 rounded px-1 py-1 text-left hover:bg-[var(--base)]"
    >
      <IconArrowLeft
        size={12}
        stroke={2}
        aria-hidden="true"
        className="shrink-0 text-[var(--ink-2)] transition-transform group-hover:-translate-x-0.5"
      />
      <span className="text-[10px] font-semibold tracking-[0.08em] text-[var(--ink-2)] uppercase">
        {name}
      </span>
    </button>
  );
}

export function SlideCard({ withClimbs = true, ...props }: BodyProps & { withClimbs?: boolean }) {
  const views: readonly View[] = withClimbs ? VIEWS : ["figures", "mix"];
  const [view, setView] = useState<View>("figures");
  const index = Math.max(views.indexOf(view), 0);
  const panes = useRef<Array<HTMLDivElement | null>>([]);
  const [height, setHeight] = useState<number>();

  // The viewport takes the height of whichever pane is showing, so the card is
  // as tall as what it is holding rather than as tall as its longest section.
  useLayoutEffect(() => {
    const pane = panes.current[index];
    if (!pane) {
      return;
    }
    setHeight(pane.getBoundingClientRect().height);
    // Again once the browser has had a frame: the mix pane holds a drawn
    // column and the climbs pane a table, and a height read in the same tick
    // they mount is short by whatever had not been laid out yet.
    const settle = requestAnimationFrame(() =>
      setHeight(panes.current[index]?.getBoundingClientRect().height),
    );

    return () => cancelAnimationFrame(settle);
  }, [index]);

  return (
    <Shell>
      <div
        className="overflow-hidden transition-[height] duration-200 ease-out"
        style={height === undefined ? undefined : { height }}
      >
        <div
          className="flex items-start transition-transform duration-200 ease-out"
          style={{
            width: `${views.length * 100}%`,
            transform: `translateX(-${index * (100 / views.length)}%)`,
          }}
        >
          <div
            ref={(node) => {
              panes.current[0] = node;
            }}
            style={{ width: `${100 / views.length}%` }}
            className="shrink-0 px-3 pt-1 pb-3"
          >
            <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5">
              <AirFigure term="Distance" value={FIGURES.distance} />
              <AirFigure term="Ascent" value={FIGURES.ascent} />
              <AirFigure term="Elevation" value={FIGURES.elevation} />
              <AirFigure term="Avg climbing" value={FIGURES.averageClimbing} />
              <AirFigure term="Max climb" value={FIGURES.steepestClimbing} icon="up" />
              <AirFigure term="Max descent" value={FIGURES.steepestDescent} icon="down" />
              <AirFigure term="Moving time" value={FIGURES.movingTime} span />
            </dl>
            <div className="mt-3 grid gap-0.5">
              <SlideRow name="Mix" summary={MIX_SUMMARY} onOpen={() => setView("mix")} />
              {withClimbs ? (
                <SlideRow name="Climbs" summary={CLIMB_SUMMARY} onOpen={() => setView("climbs")} />
              ) : null}
            </div>
          </div>
          <div
            ref={(node) => {
              panes.current[1] = node;
            }}
            style={{ width: `${100 / views.length}%` }}
            className="shrink-0 px-3 pt-1 pb-3"
          >
            <SlideHeader name={VIEW_NAMES.mix} onBack={() => setView("figures")} />
            <div className="pt-2">
              <Mixes {...props} />
            </div>
          </div>
          {withClimbs ? (
            <div
              ref={(node) => {
                panes.current[2] = node;
              }}
              style={{ width: `${100 / views.length}%` }}
              className="shrink-0 px-3 pt-1 pb-3"
            >
              <SlideHeader name={VIEW_NAMES.climbs} onBack={() => setView("figures")} />
              <div className="pt-2">
                <ClimbRows />
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </Shell>
  );
}

export { ClimbRows };

/* ---------------------------------------------------------------- G: Plain */

/**
 * A's typography with nothing left to fold.
 *
 * Once the climbs are in the dock the card holds one section, and a control
 * that only ever hides one thing is machinery earning nothing: it costs a row
 * to say what is behind it, a press to get there, and a second press to put it
 * back. Showing the mixes outright costs the height the fold was saving and
 * gives back the row, the presses and the state.
 *
 * No `useState` anywhere in it, which is the point — this is the version with
 * no interaction to get wrong.
 */
export function PlainCard({ highlight, onHighlightChange }: BodyProps) {
  return (
    <Shell>
      <div className="grid gap-3 px-3 pt-1 pb-3">
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5">
          <AirFigure term="Distance" value={FIGURES.distance} />
          <AirFigure term="Ascent" value={FIGURES.ascent} />
          <AirFigure term="Elevation" value={FIGURES.elevation} />
          <AirFigure term="Avg climbing" value={FIGURES.averageClimbing} />
          <AirFigure term="Max climb" value={FIGURES.steepestClimbing} icon="up" />
          <AirFigure term="Max descent" value={FIGURES.steepestDescent} icon="down" />
          <AirFigure term="Moving time" value={FIGURES.movingTime} span />
        </dl>
        <Mixes highlight={highlight} onHighlightChange={onHighlightChange} />
      </div>
    </Shell>
  );
}

/* ----------------------------------------------------------------- H: Rows */

/**
 * G, with the two mixes laid across instead of up.
 *
 * Side by side each column had about a hundred and sixty pixels and spent most
 * of it on the gap between a bar and its labels. Stacked across, each mix gets
 * the card's whole width, the segments are drawn at the size they actually
 * differ by, and the class sits under the ground it names.
 */
export function RowsCard({
  gapped = false,
  highlight,
  onHighlightChange,
}: BodyProps & { gapped?: boolean }) {
  return (
    <Shell>
      <div className="grid gap-3 px-3 pt-1 pb-3">
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5">
          <AirFigure term="Distance" value={FIGURES.distance} />
          <AirFigure term="Ascent" value={FIGURES.ascent} />
          <AirFigure term="Elevation" value={FIGURES.elevation} />
          <AirFigure term="Avg climbing" value={FIGURES.averageClimbing} />
          <AirFigure term="Max climb" value={FIGURES.steepestClimbing} icon="up" />
          <AirFigure term="Max descent" value={FIGURES.steepestDescent} icon="down" />
          <AirFigure term="Moving time" value={FIGURES.movingTime} span />
        </dl>
        {/*
         * Mirrored, so the two bars meet with nothing between them: gradient's
         * tags above its own bar, surface's below its own. The headings go
         * with them — five percentages say "gradient" and five surface names
         * say "surface" without being told, and two heading rows were two
         * rows spent saying what the content already said.
         */}
        <div className="grid gap-0.5">
          <MixRow
            name="Gradient"
            classesLabel="Gradient bands"
            entries={GRADIENT_MIX}
            absence="No elevation data."
            tagSide="above"
            gapped={gapped}
            highlight={highlight}
            onHighlightChange={onHighlightChange}
          />
          <MixRow
            name="Surface"
            classesLabel="Surface classes"
            entries={SURFACE_MIX}
            absence="Surface not classified yet."
            tagSide="below"
            gapped={gapped}
            highlight={highlight}
            onHighlightChange={onHighlightChange}
          />
        </div>
      </div>
    </Shell>
  );
}
