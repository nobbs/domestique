/**
 * When the ride sets off: a calendar in a popover, and a time beside it.
 *
 * A `Calendar` in a `Popover` for the day, an `Input` for the time.
 *
 * There is no default. An invented start time would draw a confident forecast
 * for a ride nobody actually planned, so it opens empty and stays that way
 * until the reader picks something.
 *
 * The window is the point. The service answers a departure whose ride
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
import {
  FORECAST_HORIZON_MS,
  FORECAST_PAST_ALLOWANCE_MS,
  startTimeRefusal,
} from "../lib/startTime";
import { Button } from "./Button";

const TIME_ID = "start-time-of-day";

function pad(value: number): string {
  return String(value).padStart(2, "0");
}

/** `HH:MM`, which is what `input[type=time]` reads and writes. */
function toTimeValue(date: Date): string {
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function startOfDay(date: Date): Date {
  const next = new Date(date);
  next.setHours(0, 0, 0, 0);

  return next;
}

function endOfDay(date: Date): Date {
  const next = new Date(date);
  next.setHours(23, 59, 59, 999);

  return next;
}

/** The day from one date and the time of day from another. */
function combine(day: Date, timeOf: Date): Date {
  const next = new Date(day);
  next.setHours(timeOf.getHours(), timeOf.getMinutes(), 0, 0);

  return next;
}

export function StartTimePicker({
  value,
  onChange,
  movingSeconds,
  inline = false,
}: {
  value: Date | null;
  onChange: (next: Date | null) => void;
  /** The ride's length, since the horizon applies to when it *finishes*. */
  movingSeconds?: number | undefined;
  /**
   * Drops the stacked label and sits on one line.
   *
   * For the forecast's own caption row, where the surrounding words already
   * say what the control is for — "Forecast from …" reads as a sentence, and a
   * second "Ride start" above it would be labelling the label.
   */
  inline?: boolean;
}) {
  const [refusal, setRefusal] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  /*
   * A day the reader has picked while no departure exists yet. Held rather
   * than proposed: there is deliberately no default start time, and proposing
   * the day at midnight would draw a confident forecast for a ride nobody
   * planned. The departure is proposed once the time field fills the other
   * half in. There is no clear: a start once set can only be changed.
   */
  const [pendingDay, setPendingDay] = useState<Date | null>(null);
  const now = new Date();
  /*
   * The window's own edges carry a time of day, and a calendar offers days.
   * Rounding them outwards to whole days is what keeps a day with valid hours
   * in it selectable — the edge of the horizon falls at, say, half past two,
   * and disabling that whole day would refuse the morning along with the
   * evening. Which moments on it are allowed stays `startTimeRefusal`'s to
   * say, and it is asked again about whatever the two halves assemble.
   */
  const earliest = startOfDay(new Date(now.getTime() - FORECAST_PAST_ALLOWANCE_MS));
  const latest = endOfDay(
    new Date(now.getTime() + FORECAST_HORIZON_MS - Math.max(movingSeconds ?? 0, 0) * 1000),
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

  /**
   * Proposes whatever the time field holds, from either the event that reports
   * it. WebKit fires neither `input` nor `change` while a time field's segments
   * are being edited, so a departure typed into one and left focused reached
   * nothing: the reader saw a day, a time, and no forecast.
   */
  const proposeTime = (raw: string) => {
    // Clearing the field is not a proposal. An empty string splits to [""] and
    // Number("") is 0, so without this the guard below reads a cleared field —
    // or an unfilled one blurred past — as a confident midnight.
    if (raw === "") {
      return;
    }
    const [hours, minutes] = raw.split(":").map(Number);
    const day = value ?? pendingDay;
    if (day === null || hours === undefined || Number.isNaN(hours)) {
      return;
    }
    const next = new Date(day);
    next.setHours(hours, minutes ?? 0, 0, 0);
    // Where both events do arrive, the second says nothing the first did not:
    // every focus and blur of an untouched field would otherwise hand the page
    // a fresh Date and re-render everything hanging off the departure.
    if (value !== null && next.getTime() === value.getTime()) {
      return;
    }
    propose(next);
  };

  // The chosen departure's day, or the day waiting for its time.
  const shownDay = value ?? pendingDay;
  // A day alone is not a departure, and the control reads the same either way.
  const hint = value === null && pendingDay !== null ? "Add a time to forecast this ride." : null;

  const controls = (
    <div className="flex items-center gap-1.5">
      {/*
       * The application's own `Button`, not `ui/button`: the shadcn one is the
       * vocabulary the generated components speak among themselves, and this
       * file is application code, which lint enforces.
       */}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              variant="outline"
              icon={<IconCalendar stroke={1.8} />}
              className={
                inline ? "h-7 px-2 text-xs font-normal tabular-nums" : "font-normal tabular-nums"
              }
              aria-label="The day the ride starts"
            />
          }
        >
          {shownDay === null
            ? "Pick a day"
            : shownDay.toLocaleDateString(undefined, {
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
            {...(shownDay === null ? {} : { selected: shownDay, defaultMonth: shownDay })}
            // Offered days are the ones the service would actually answer for.
            disabled={{ before: earliest, after: latest }}
            onSelect={(day) => {
              if (day === undefined) {
                return;
              }
              if (value === null) {
                setPendingDay(day);
                setRefusal(null);
              } else {
                propose(combine(day, value));
              }
              setOpen(false);
            }}
            autoFocus
          />
        </PopoverContent>
      </Popover>
      <Input
        id={TIME_ID}
        type="time"
        // Typed, not picked: the field takes hours and minutes from the keys,
        // and WebKit's clock button opened a dropdown styled by nobody here.
        // Five digit-widths for "hh:mm" plus the padding, so the box fits its text.
        className={`appearance-none tabular-nums [&::-webkit-calendar-picker-indicator]:hidden ${inline ? "h-7 w-[calc(5ch+1.25rem)] px-2 text-xs" : "w-[calc(5ch+1.75rem)]"}`}
        aria-label="The time the ride starts"
        aria-describedby={refusal || hint ? "start-time-refusal" : undefined}
        // Nothing for it to set a time on: no day is chosen and none pending,
        // so an enabled field would be a control that swallows keystrokes.
        disabled={shownDay === null}
        value={value ? toTimeValue(value) : ""}
        onChange={(event) => proposeTime(event.target.value)}
        onBlur={(event) => proposeTime(event.target.value)}
      />
    </div>
  );

  if (inline) {
    return (
      <div className="flex items-center gap-1.5">
        {controls}
        {(refusal ?? hint) ? (
          <span
            id="start-time-refusal"
            // What FieldError gives the block form for free: the refusal is the
            // answer to what the reader just did, and has to be spoken as one.
            // The hint is not: nothing has been refused, and half a departure
            // is a state to describe rather than an error to announce.
            {...(refusal ? { role: "alert" } : {})}
            className={
              refusal
                ? "text-[11px] text-[var(--danger,var(--ink-2))]"
                : "text-[11px] text-[var(--ink-2)]"
            }
          >
            {refusal ?? hint}
          </span>
        ) : null}
      </div>
    );
  }

  return (
    <Field>
      <FieldLabel className="font-semibold" htmlFor={TIME_ID}>
        Ride start
      </FieldLabel>
      {controls}
      {refusal ? <FieldError id="start-time-refusal">{refusal}</FieldError> : null}
      {refusal === null && hint !== null ? (
        <p id="start-time-refusal" className="text-xs text-[var(--ink-2)]">
          {hint}
        </p>
      ) : null}
    </Field>
  );
}
