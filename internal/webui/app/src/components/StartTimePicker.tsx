/**
 * When the ride starts, which is what turns a stage's predicted moving time
 * into real moments worth asking `GET /v1/weather` about.
 *
 * A `ui/field` around a `ui/input`, like every other labelled control here. It
 * takes a value and a callback, with nothing held here.
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
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  FORECAST_HORIZON_MS,
  FORECAST_PAST_ALLOWANCE_MS,
  startTimeRefusal,
} from "../lib/startTime";

const INPUT_ID = "start-time-input";

/** The next whole minute at or after `epochMs`, since the input works in minutes. */
function ceilToMinute(epochMs: number): number {
  return Math.ceil(epochMs / 60_000) * 60_000;
}

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
  movingSeconds?: number | undefined;
}

export function StartTimePicker({ value, onChange, movingSeconds }: StartTimePickerProps) {
  const [refusal, setRefusal] = useState<string | null>(null);
  const now = new Date();
  const latestStart = new Date(
    now.getTime() + FORECAST_HORIZON_MS - Math.max(movingSeconds ?? 0, 0) * 1000,
  );

  return (
    <Field>
      <FieldLabel className="font-semibold" htmlFor={INPUT_ID}>
        Ride start
      </FieldLabel>
      <Input
        id={INPUT_ID}
        type="datetime-local"
        // The browser's own hint for the same window this refuses by hand. Computed
        // once per render rather than kept ticking; the handler reads the clock afresh,
        // so a drifted hint costs a refusal rather than a bad request. Rounded up to the
        // next whole minute, since truncating would advertise a minute already too old.
        min={toInputValue(new Date(ceilToMinute(now.getTime() - FORECAST_PAST_ALLOWANCE_MS)))}
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
          // A fresh reading rather than the one this render captured: a page
          // can sit open for hours, and the window this is checked against is
          // the one in force when the reader picks, not when the field drew.
          const asOf = new Date();
          // Classified by the same function the page uses on a remembered
          // value, so a keystroke and a stored time can never be told two
          // different things about the same trouble.
          const refusal = startTimeRefusal(parsed, movingSeconds, asOf);
          if (refusal !== null) {
            setRefusal(
              refusal === "past"
                ? "That's more than a day in the past."
                : "That ride would finish past the 16-day forecast horizon.",
            );

            return;
          }
          setRefusal(null);
          onChange(parsed);
        }}
      />
      {refusal ? <FieldError id="start-time-refusal">{refusal}</FieldError> : null}
    </Field>
  );
}
