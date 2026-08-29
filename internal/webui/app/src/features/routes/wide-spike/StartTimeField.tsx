/**
 * When the ride sets off: a calendar in a popover, and a time beside it.
 *
 * The panel already has a departure control — `StartTimePicker`, a native
 * `datetime-local` field — and this is the shadcn one instead: a `Calendar` in
 * a `Popover` for the day, an `Input` for the time. It is a spike of the
 * control's *look*, so it deliberately keeps everything the existing one knows
 * and changes only how the day is chosen.
 *
 * What it keeps is the window. The service answers a departure whose ride
 * would finish past the sixteen-day horizon with a `400`, so the calendar
 * disables those days rather than offering them, and `startTimeRefusal` — the
 * same function the page uses on a remembered value — has the last word on
 * whatever is assembled. A picker that looked better and quietly allowed a
 * request the service refuses would be a worse control, not a nicer one.
 *
 * The two halves are one value. A day without a time is not a departure, so
 * the time carries on from whatever was set and only the date moves; changing
 * either re-checks the pair.
 */

import { IconCalendar } from "@tabler/icons-react";
import { useState } from "react";
import { Calendar } from "@/components/ui/calendar";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Button } from "../../../components/Button";
import {
  FORECAST_HORIZON_MS,
  FORECAST_PAST_ALLOWANCE_MS,
  startTimeRefusal,
} from "../../../lib/startTime";

const TIME_ID = "start-time-of-day";

function pad(value: number): string {
  return String(value).padStart(2, "0");
}

/** `HH:MM`, which is what `input[type=time]` reads and writes. */
function toTimeValue(date: Date): string {
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** The day from one date and the time of day from another. */
function combine(day: Date, timeOf: Date): Date {
  const next = new Date(day);
  next.setHours(timeOf.getHours(), timeOf.getMinutes(), 0, 0);

  return next;
}

export function StartTimeField({
  value,
  onChange,
  movingSeconds,
}: {
  value: Date | null;
  onChange: (next: Date) => void;
  /** The ride's length, since the horizon applies to when it *finishes*. */
  movingSeconds?: number | undefined;
}) {
  const [refusal, setRefusal] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const now = new Date();
  const earliest = new Date(now.getTime() - FORECAST_PAST_ALLOWANCE_MS);
  const latest = new Date(
    now.getTime() + FORECAST_HORIZON_MS - Math.max(movingSeconds ?? 0, 0) * 1000,
  );

  /** Proposes a departure, and says why if the service would not take it. */
  const propose = (next: Date) => {
    // Read afresh rather than from this render: a page can sit open for hours,
    // and the window that matters is the one in force when the reader picks.
    const complaint = startTimeRefusal(next, movingSeconds, new Date());
    if (complaint !== null) {
      setRefusal(
        complaint === "past"
          ? "That's more than a day in the past."
          : "That ride would finish past the 16-day forecast horizon.",
      );

      return;
    }
    setRefusal(null);
    onChange(next);
  };

  return (
    <Field>
      <FieldLabel className="font-semibold" htmlFor={TIME_ID}>
        Ride start
      </FieldLabel>
      <div className="flex items-center gap-2">
        <Popover open={open} onOpenChange={setOpen}>
          {/*
           * The application's own `Button`, not `ui/button`: the shadcn one is
           * the vocabulary the generated components speak among themselves,
           * and this file is application code, which lint enforces.
           */}
          <PopoverTrigger
            render={
              <Button
                variant="outline"
                icon={<IconCalendar stroke={1.8} />}
                className="font-normal tabular-nums"
              />
            }
          >
            {value === null
              ? "Pick a day"
              : value.toLocaleDateString(undefined, {
                  day: "numeric",
                  month: "short",
                  year: "numeric",
                })}
          </PopoverTrigger>
          <PopoverContent align="start" className="w-auto p-0">
            <Calendar
              mode="single"
              // Spread only when there is one. Under
              // `exactOptionalPropertyTypes` an optional prop and one that may
              // be undefined are different types, and `react-day-picker`
              // declares these as the former.
              {...(value === null ? {} : { selected: value, defaultMonth: value })}
              // Offered days are the ones the service would actually answer for.
              disabled={{ before: earliest, after: latest }}
              onSelect={(day) => {
                if (day === undefined) {
                  return;
                }
                propose(combine(day, value ?? day));
                setOpen(false);
              }}
              autoFocus
            />
          </PopoverContent>
        </Popover>
        <Input
          id={TIME_ID}
          type="time"
          className="w-[7.5rem] tabular-nums"
          aria-describedby={refusal ? "start-time-refusal" : undefined}
          value={value ? toTimeValue(value) : ""}
          onChange={(event) => {
            const raw = event.target.value;
            const [hours, minutes] = raw.split(":").map(Number);
            if (value === null || hours === undefined || Number.isNaN(hours)) {
              return;
            }
            const next = new Date(value);
            next.setHours(hours, minutes ?? 0, 0, 0);
            propose(next);
          }}
        />
      </div>
      {refusal ? <FieldError id="start-time-refusal">{refusal}</FieldError> : null}
    </Field>
  );
}
