/**
 * The choice of what to wash the route in, and the key for what was chosen —
 * two components rather than one, so a caller can place the choice beside the
 * departure time and the key beside the strip without the two dragging each
 * other along.
 *
 * `ConditionsKey`'s bands are the "keyed" look from the dock spike: a short
 * bar for a corridor band, a thin stroke for a route-line one, each a
 * tooltip trigger so the cut it means waits under the pointer rather than
 * crowding a key that has to fit on one line.
 *
 * Wind gets a second group in that line, because it is drawn twice: a
 * corridor for how hard it blows, and the route itself for what that does to
 * the rider.
 */

import { Tooltip } from "@base-ui/react/tooltip";
import { IconCircleOff } from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { ForecastSample } from "../../lib/forecastSamples";
import type { Measure, MeasureKey } from "../../lib/measures";
import {
  bandRange,
  MEASURES,
  measureVariable,
  WIND_RELATION_KEY,
  windRelationVariable,
} from "../../lib/measures";

const CHOICE =
  "flex items-center gap-1 rounded-full border border-[var(--rule)] px-2 py-0.5 text-[11px] leading-none text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] disabled:pointer-events-none disabled:opacity-50 aria-pressed:border-[var(--accent)] aria-pressed:font-semibold aria-pressed:text-[var(--ink)]";

export interface ConditionsChoicesProps {
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

/** The Off + measures button group, each choice named and iconed. */
export function ConditionsChoices({
  measure,
  onMeasureChange,
  samples,
  movingSeconds,
}: ConditionsChoicesProps) {
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
          <IconCircleOff size={12} stroke={2} aria-hidden="true" />
          Off
        </button>
        {MEASURES.map((entry) => {
          const Icon = entry.icon;
          return (
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
              <Icon size={12} stroke={2} aria-hidden="true" />
              {entry.label}
            </button>
          );
        })}
      </div>
      {available ? null : <p className="text-[11px] text-[var(--ink-2)]">{absence}</p>}
    </div>
  );
}

const TRIGGER =
  "flex items-center gap-1 rounded px-0.5 text-[11px] leading-none text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]";

/** A tooltip-triggering key entry: a mark and its label. */
function Entry({ mark, label, detail }: { mark: ReactNode; label: string; detail: string }) {
  return (
    <li>
      <Tooltip.Root>
        <Tooltip.Trigger className={TRIGGER}>
          {mark}
          {label}
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Positioner sideOffset={6}>
            {/* This build of Base UI leaves the popup's role to its consumer. */}
            <Tooltip.Popup
              role="tooltip"
              className="max-w-56 rounded-md bg-[var(--ink)] px-2 py-1 text-[11px] text-[var(--panel)] shadow-[var(--shadow)]"
            >
              {detail}
            </Tooltip.Popup>
          </Tooltip.Positioner>
        </Tooltip.Portal>
      </Tooltip.Root>
    </li>
  );
}

/** A corridor band: a short filled bar, empty and outlined at zero opacity. */
function Bar({ colour, opacity }: { colour: string; opacity: number }) {
  return (
    <span
      aria-hidden="true"
      className="h-2 w-4 rounded-full border border-[var(--rule)]"
      style={{ backgroundColor: `color-mix(in srgb, ${colour} ${opacity * 100}%, transparent)` }}
    />
  );
}

/** A route-line band: a thin stroke in the band's own colour. */
function Stroke({ colour }: { colour: string }) {
  return (
    <span
      aria-hidden="true"
      className="h-0.5 w-4 rounded-full"
      style={{ backgroundColor: colour }}
    />
  );
}

export interface ConditionsKeyProps {
  measure: MeasureKey | null;
  samples: ForecastSample[];
}

/** The key for one measure's wash, on one line: the corridor, and for wind the route line too. */
export function ConditionsKey({ measure, samples }: ConditionsKeyProps) {
  const chosen: Measure | undefined = MEASURES.find((entry) => entry.key === measure);
  if (samples.length === 0) {
    return null;
  }
  if (chosen === undefined) {
    // Holds the key's row while the wash is off, so the strip above keeps its height.
    return (
      <div className="text-[11px]" aria-hidden="true">
        &nbsp;
      </div>
    );
  }

  return (
    <Tooltip.Provider>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-[var(--ink-2)]">
        <div className="flex items-center gap-x-1.5">
          <span className="text-[10px] tracking-[0.06em] text-[var(--ink-2)] uppercase">
            Corridor
          </span>
          <ul className="flex items-center gap-x-1.5">
            {chosen.bands.map((band, index) => {
              const opacity = chosen.opacity(index);
              return (
                <Entry
                  key={band.label}
                  mark={<Bar colour={measureVariable(chosen.key, index)} opacity={opacity} />}
                  label={band.label}
                  detail={`${band.description} · ${bandRange(chosen, index)}${opacity === 0 ? " · not washed" : ""}`}
                />
              );
            })}
          </ul>
        </div>
        {chosen.key === "wind" ? (
          <div className="flex items-center gap-x-1.5">
            <span className="text-[10px] tracking-[0.06em] text-[var(--ink-2)] uppercase">
              Route line
            </span>
            <ul className="flex items-center gap-x-1.5">
              {WIND_RELATION_KEY.map((band) => (
                <Entry
                  key={band.description}
                  mark={<Stroke colour={windRelationVariable(band.stop)} />}
                  label={band.label}
                  detail={`${band.description} · replaces the steepness edging`}
                />
              ))}
            </ul>
          </div>
        ) : null}
      </div>
    </Tooltip.Provider>
  );
}
