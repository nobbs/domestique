/**
 * Fitness: whether the training is working, and how much more of it there is room for.
 *
 * Four figures lead — fitness, ramp rate, this week's building range and form —
 * then one chart runs the range straight into three projected weeks, and the
 * evidence that the fitness is real follows: best power against the window
 * before, decoupling, and time in zone. Both load scales stay, because neither
 * converts to the other; the toggle picks the one being read.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { activitiesQuery, fitnessQuery, webUIConfigQuery } from "../../api/queries";
import { formatCalendarDay } from "../../components/chart/TimeFrame";
import { PageShell } from "../../components/Layout";
import { Skeleton } from "../../components/ui/skeleton";
import { DecouplingPanel, DecouplingSummary, decouplingRides } from "./DecouplingPanel";
import { FitnessSection, FitnessStat } from "./FitnessSection";
import {
  bandOf,
  FORM_BANDS,
  formPercent,
  mondayOf,
  RAMP_BANDS,
  type Reading,
  reading,
  type Scale,
  scaleOutlook,
  signed,
  weeklyLoads,
} from "./form";
import { PowerDuration } from "./PowerDuration";
import { SeasonChart } from "./SeasonChart";
import { ZonePanel } from "./ZonePanel";

const SCALES: ReadonlyArray<{ value: Scale; label: string; name: string; unit: string }> = [
  { value: "tss", label: "TSS", name: "stress score", unit: "TSS" },
  { value: "trimp", label: "TRIMP", name: "TRIMP", unit: "TRIMP" },
];

/** How far back the page opens, and what the range control offers. */
const RANGES: ReadonlyArray<{ value: string; label: string; days: number }> = [
  { value: "90", label: "3 months", days: 90 },
  { value: "180", label: "6 months", days: 180 },
  { value: "365", label: "1 year", days: 365 },
];

// Only while the config is unavailable: once it answers, its zone is the one used.
const browserZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

function Toggle<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: ReadonlyArray<{ value: T; label: string }>;
  onChange: (next: T) => void;
}) {
  return (
    <ToggleGroup
      aria-label={label}
      variant="outline"
      size="sm"
      spacing={0}
      value={[value]}
      onValueChange={(next) => {
        // Pressing the pressed one empties the group; the page is always on one choice.
        const chosen = options.find((option) => option.value === next[0]);
        if (chosen) {
          onChange(chosen.value);
        }
      }}
    >
      {options.map((option) => (
        <ToggleGroupItem key={option.value} value={option.value}>
          {option.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

/** How many days of rest bring form into the fresh band, as the note under form says it. */
function restNote(today: Reading, restForm: readonly number[]): string {
  const fresh = bandOf(FORM_BANDS, today.formPercent);
  if (today.formPercent >= 5) {
    return `${fresh.name} · ${fresh.advice}`;
  }
  const days = restForm.findIndex((percent) => percent >= 5);
  if (days < 0) {
    return `${fresh.name} · not fresh within three weeks of rest`;
  }

  return `${fresh.name} · fresh after ${days + 1} ${days === 0 ? "day" : "days"} of rest`;
}

export function FitnessPage() {
  const [scale, setScale] = useState<Scale>("tss");
  const [range, setRange] = useState("180");
  const selected = RANGES.find(({ value }) => value === range) ?? RANGES[1];
  const days = selected?.days ?? 180;
  const from = useMemo(() => {
    const start = new Date();
    start.setDate(start.getDate() - days);

    return start.toISOString();
  }, [days]);
  const { data, isPending, isError } = useQuery(fitnessQuery({ from }));
  const activities = useQuery(activitiesQuery());
  const config = useQuery(webUIConfigQuery());
  const zone = config.data?.timezone || browserZone();
  const rides = useMemo(
    () => decouplingRides(activities.data ?? [], zone),
    [activities.data, zone],
  );
  const readings = useMemo(
    () => (data?.days ?? []).map((day) => reading(day, scale)),
    [data, scale],
  );
  const scaleInfo = SCALES.find(({ value }) => value === scale) ?? SCALES[0];
  const today = readings[readings.length - 1];
  // An outlook projected from any day but the last one shown would not join the line it continues.
  const outlook =
    data?.outlook && data.outlook.date === today?.date
      ? scaleOutlook(data.outlook, scale)
      : undefined;
  const dates = readings.map((one) => one.date);

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-5">
        <header className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h1 className="font-semibold text-2xl tracking-tight">Fitness</h1>
            {today ? (
              <p className="text-[var(--ink-2)] text-sm">
                As of {formatCalendarDay(today.date)}, on the {scaleInfo?.name} scale
              </p>
            ) : null}
          </div>
          {readings.length > 0 ? (
            <div className="flex flex-wrap items-center gap-2">
              <Toggle label="Range" value={range} options={RANGES} onChange={setRange} />
              <Toggle label="Scale" value={scale} options={SCALES} onChange={setScale} />
            </div>
          ) : null}
        </header>
        {isPending ? (
          <Skeleton className="h-64 w-full" role="status" aria-label="Loading the timeline" />
        ) : isError ? (
          <p className="text-sm text-[var(--alert)]" role="alert">
            The service did not say what the training has come to.
          </p>
        ) : !today ? (
          <p className="text-[var(--ink-2)] text-sm">
            Nothing has been worked out yet. Training load needs the numbers on{" "}
            <Link className="underline" to="/settings">
              settings
            </Link>
            , and a ride recorded with a heart-rate strap or a power meter.
          </p>
        ) : (
          <>
            <Headline
              readings={readings}
              today={today}
              outlook={outlook}
              unit={scaleInfo?.unit ?? ""}
            />
            <FitnessSection title="Fitness, form and load">
              <SeasonChart
                readings={readings}
                outlook={outlook}
                scaleName={scaleInfo?.name ?? ""}
              />
            </FitnessSection>
            {data.powerCurve && data.powerCurve.length > 0 ? (
              <FitnessSection title="Power duration">
                <PowerDuration
                  current={data.powerCurve}
                  previous={data.powerCurvePrevious}
                  previousName={`the ${selected?.label ?? "range"} before`}
                />
                <p className="text-[var(--ink-2)] text-xs">
                  The best each duration reached, from measured power alone. A duration no ride was
                  long enough for carries no point.
                </p>
              </FitnessSection>
            ) : null}
            <div className="grid gap-5 md:grid-cols-2">
              {activities.isError ? (
                // An outage must not read as a season with nothing in it.
                <p className="text-sm text-[var(--alert)]" role="alert">
                  The service did not say what has been ridden, so the season's decoupling is not
                  drawn.
                </p>
              ) : rides.some((ride) => ride.date >= (dates[0] ?? "")) ? (
                <FitnessSection
                  title="Decoupling"
                  aside={<DecouplingSummary rides={rides} dates={dates} />}
                >
                  <DecouplingPanel rides={rides} dates={dates} />
                </FitnessSection>
              ) : null}
              {data.weeks.length > 0 ? (
                <FitnessSection title="Time in zone">
                  <ZonePanel weeks={data.weeks} dates={dates} />
                </FitnessSection>
              ) : null}
            </div>
          </>
        )}
      </div>
    </PageShell>
  );
}

function Headline({
  readings,
  today,
  outlook,
  unit,
}: {
  readings: readonly Reading[];
  today: Reading;
  outlook: ReturnType<typeof scaleOutlook> | undefined;
  unit: string;
}) {
  const start = readings[0] ?? today;
  const peak = readings.reduce((best, one) => (one.fitness > best.fitness ? one : best), today);
  const formBand = bandOf(FORM_BANDS, today.formPercent);
  const rest = outlook?.plans.find((plan) => plan.plan === "rest");
  const restForm = rest?.days.map((one) => formPercent(one.form, one.fitness)) ?? [];

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <FitnessStat
        label="Fitness"
        value={today.fitness.toFixed(0)}
        note={`${signed(today.fitness - start.fitness)} since ${formatCalendarDay(start.date)} · peak ${peak.fitness.toFixed(0)}`}
      />
      {outlook ? <Ramp ramp={outlook.rampPerWeek} fitness={today.fitness} /> : null}
      {outlook ? (
        <FitnessStat
          label="This week, to keep building"
          value={`${Math.round(outlook.weekLoadLow)}–${Math.round(outlook.weekLoadHigh)}`}
          unit={unit}
          note={`${Math.round(weeklyLoads(readings).get(mondayOf(today.date)) ?? 0)} so far · last 4 weeks averaged ${Math.round(outlook.habitualDailyLoad * 7)}`}
        />
      ) : null}
      <FitnessStat
        label="Form"
        value={`${signed(today.formPercent)}%`}
        unit="of fitness"
        tone={formBand.colour}
        note={outlook ? restNote(today, restForm) : `${formBand.name} · ${formBand.advice}`}
      />
    </div>
  );
}

function Ramp({ ramp, fitness }: { ramp: number; fitness: number }) {
  const before = fitness - ramp;
  const percent = before > 1 ? (ramp / before) * 100 : 0;
  const band = bandOf(RAMP_BANDS, percent);

  return (
    <FitnessStat
      label="Ramp rate"
      value={signed(ramp, 1)}
      unit="a week"
      tone={band.colour}
      note={`${band.name} · ${signed(percent)}% of fitness`}
    />
  );
}
