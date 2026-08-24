/**
 * When the ride starts, which is what turns a stage's predicted moving time
 * into real moments worth asking `GET /v1/weather` about.
 *
 * The first date input in this codebase, so it gets a proper `<label>` rather
 * than borrowing a pattern that does not exist yet. Built like `UnitPicker`:
 * a value and a callback, nothing held here.
 *
 * There is no default. An invented start time would draw a confident forecast
 * for a ride nobody actually planned, so the field opens empty and stays that
 * way until the reader picks something.
 *
 * A time outside `isWithinForecastWindow` is refused rather than sent: the
 * endpoint answers one with a `400 invalid_request`, and a control that can
 * check the window itself has no business making that round trip. Refusing
 * leaves the previous value in place — a controlled input re-renders with the
 * prop it was given, so an invalid keystroke never reaches `onChange`.
 */

import { useState } from "react";
import {
  FORECAST_HORIZON_MS,
  FORECAST_PAST_ALLOWANCE_MS,
  isWithinForecastWindow,
} from "../lib/startTime";

const INPUT_ID = "start-time-input";

function pad(value: number): string {
  return String(value).padStart(2, "0");
}

/** `datetime-local`'s own wire format, in the browser's local time. */
function toInputValue(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export interface StartTimePickerProps {
  value: Date | null;
  onChange: (next: Date | null) => void;
  /**
   * The stage's predicted moving time, when it has one.
   *
   * The horizon belongs to the *last* sample, not the first: the forecast
   * request spans from the departure to the arrival, and a five-hour ride
   * begun at the very edge of the window ends five hours past it. Bounding the
   * control by the horizon alone would offer a start that is certain to be
   * refused, and the refusal would arrive as a `400` the page can only report
   * as the provider being unavailable — which it would not be.
   */
  rideSeconds?: number | undefined;
}

export function StartTimePicker({ value, onChange, rideSeconds }: StartTimePickerProps) {
  const [refusal, setRefusal] = useState<string | null>(null);
  const now = new Date();
  const latestStart = new Date(
    now.getTime() + FORECAST_HORIZON_MS - Math.max(rideSeconds ?? 0, 0) * 1000,
  );

  return (
    <div className="start-time-picker">
      <label className="start-time-picker__label" htmlFor={INPUT_ID}>
        Ride start
      </label>
      <input
        id={INPUT_ID}
        className="start-time-picker__input"
        type="datetime-local"
        // The browser's own hint for the same window this refuses by hand —
        // cheap to offer and it steers a picker away from the refusal before
        // the reader ever reaches it. Computed once per render rather than
        // kept ticking live, which a control open for a whole session would
        // need; nobody minds the boundary drifting by the odd minute.
        min={toInputValue(new Date(now.getTime() - FORECAST_PAST_ALLOWANCE_MS))}
        max={toInputValue(latestStart)}
        value={value ? toInputValue(value) : ""}
        aria-describedby={refusal ? "start-time-refusal" : undefined}
        onChange={(event) => {
          const raw = event.target.value;
          if (raw === "") {
            setRefusal(null);
            onChange(null);

            return;
          }
          const parsed = new Date(raw);
          if (Number.isNaN(parsed.getTime())) {
            return;
          }
          // The arrival is what the forecast request has to reach, so it is
          // what the window is checked against.
          const arrival = new Date(parsed.getTime() + Math.max(rideSeconds ?? 0, 0) * 1000);
          if (!isWithinForecastWindow(parsed, now) || !isWithinForecastWindow(arrival, now)) {
            setRefusal(
              parsed.getTime() > now.getTime()
                ? "That ride would finish past the 16-day forecast horizon."
                : "That's more than a day in the past.",
            );

            return;
          }
          setRefusal(null);
          onChange(parsed);
        }}
      />
      {refusal ? (
        <p className="start-time-picker__refusal" id="start-time-refusal" role="alert">
          {refusal}
        </p>
      ) : null}
    </div>
  );
}
