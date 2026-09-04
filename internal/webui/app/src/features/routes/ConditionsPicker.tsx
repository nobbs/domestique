/**
 * Which forecast measure the map is washed in, and what the wash means.
 *
 * In the dock rather than in the map's corner. That corner already carries
 * zoom, locate, compass and basemap — controls for the *view* — while every
 * other forecast control already lives down here beside the departure and the
 * strip. It also keeps the forecast out of `LibraryMap`, which the library
 * shares and which has no forecast at all.
 *
 * One exclusive choice, iterated over `MEASURES` so a fifth measure appears
 * here by being added to the registry. Off is a choice of its own and the
 * default: an overlay that tints the ground before anybody asked is an overlay
 * covering the map.
 *
 * The legend's swatches are custom properties, not the hex `measures.ts` hands
 * MapLibre — nothing here is on the map, so it follows the page's theme, which
 * is the split `mix.ts` states for the steepness and ground keys. Every band is
 * named as well as coloured; a key that only shows colours is unreadable to the
 * readers this legend exists for.
 *
 * Wind gets two keys, because it is drawn twice: a corridor for how hard it
 * blows, and the route itself for what that does to the rider.
 */

import type { ForecastSample } from "../../lib/forecastSamples";
import type { Measure, MeasureKey } from "../../lib/measures";
import {
  MEASURES,
  measureVariable,
  WIND_RELATION_KEY,
  windRelationVariable,
} from "../../lib/measures";

/** The pill every choice wears, pressed or not. */
const CHOICE =
  "rounded-full border border-[var(--rule)] px-2 py-0.5 text-[11px] leading-none text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] disabled:pointer-events-none disabled:opacity-50 aria-pressed:border-[var(--accent)] aria-pressed:font-semibold aria-pressed:text-[var(--ink)]";

/**
 * One row of the key: a swatch and the words for what it means.
 *
 * `opacity` blends into the fill alone via `color-mix()`, not the element's
 * own CSS opacity — that would fade the border too, erasing a "not washed"
 * band's outline along with its colour.
 */
function Swatch({ colour, opacity = 1 }: { colour: string; opacity?: number }) {
  return (
    <span
      aria-hidden="true"
      className="h-2.5 w-4 rounded-xs border border-[var(--rule)]"
      style={{ backgroundColor: `color-mix(in srgb, ${colour} ${opacity * 100}%, transparent)` }}
    />
  );
}

/**
 * What the route line itself is drawn in while the wind is on show, which is a
 * second thing to say and not a second colour for the same one: the corridor
 * carries the wind's speed, the route carries what it does to the rider.
 *
 * It also says plainly that the steepness edging has stood down. The map cannot
 * carry two ramps along one line, so the reader is told which one is on rather
 * than left to notice that the gradient colours went away.
 */
function WindRelationKey() {
  return (
    <div className="grid gap-1">
      <p className="text-[11px] text-[var(--ink-2)]">
        The route itself shows the wind against the way you are riding, in place of its steepness
        edging.
      </p>
      <ul className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-[var(--ink-2)]">
        {WIND_RELATION_KEY.map((band) => (
          <li key={band.description} className="flex items-center gap-1.5">
            <Swatch colour={windRelationVariable(band.stop)} />
            {band.description}
          </li>
        ))}
      </ul>
    </div>
  );
}

function Legend({ measure }: { measure: Measure }) {
  return (
    <ul className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-[var(--ink-2)]">
      {measure.bands.map((band, index) => (
        <li key={band.label} className="flex items-center gap-1.5">
          {/*
           * The opacity is exactly what the map paints, so a band that washes
           * nothing — rain's dry, cloud's clear — shows here as an empty swatch
           * rather than as a colour the route never wears.
           */}
          <Swatch colour={measureVariable(measure.key, index)} opacity={measure.opacity(index)} />
          {band.description}
          {measure.opacity(index) === 0 ? " (not washed)" : null}
        </li>
      ))}
    </ul>
  );
}

export interface ConditionsPickerProps {
  /** The measure the reader asked for, and null — the default — for none. */
  measure: MeasureKey | null;
  onMeasureChange: (next: MeasureKey | null) => void;
  /**
   * The forecast requests for this ride, which is what there is to wash the
   * route in. Empty leaves every choice inert with a line saying why.
   */
  samples: ForecastSample[];
  /** The ride's predicted moving time, absent for a route nothing has predicted. */
  movingSeconds?: number | undefined;
}

export function ConditionsPicker({
  measure,
  onMeasureChange,
  samples,
  movingSeconds,
}: ConditionsPickerProps) {
  const chosen = MEASURES.find((entry) => entry.key === measure) ?? null;
  const available = samples.length > 0;
  // Two different absences, and only one of them is the reader's to fix.
  const absence =
    movingSeconds === undefined
      ? "Nothing has predicted a moving time for this route, so there is no forecast to wash it in."
      : "Pick a ride start, and the route can be washed in its forecast.";

  return (
    <div className="grid gap-1.5">
      <div
        role="group"
        aria-label="Conditions washed along the route"
        className="flex flex-wrap items-center gap-1"
      >
        <button
          type="button"
          aria-pressed={measure === null}
          disabled={!available}
          onClick={() => onMeasureChange(null)}
          className={CHOICE}
        >
          Off
        </button>
        {MEASURES.map((entry) => (
          <button
            key={entry.key}
            type="button"
            aria-pressed={measure === entry.key}
            disabled={!available}
            // Pressing the pressed one is the way back out, the same gesture
            // the ground key offers for a class already picked.
            onClick={() => onMeasureChange(measure === entry.key ? null : entry.key)}
            className={CHOICE}
          >
            {entry.label}
          </button>
        ))}
      </div>
      {available ? null : <p className="text-[11px] text-[var(--ink-2)]">{absence}</p>}
      {available && chosen ? <Legend measure={chosen} /> : null}
      {available && chosen?.key === "wind" ? <WindRelationKey /> : null}
    </div>
  );
}
