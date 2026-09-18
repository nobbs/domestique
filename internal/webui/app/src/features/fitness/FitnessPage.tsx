/**
 * Fitness: whether the training is working, and how much more of it there is room for.
 *
 * Four figures lead — fitness, ramp rate, this week's building range and form —
 * then one chart runs the range straight into three projected weeks, and the
 * evidence that the fitness is real follows: best power against the window
 * before, decoupling, and time in zone. Both load scales stay, because neither
 * converts to the other; the toggle picks the one being read.
 */

import {
  IconActivityHeartbeat,
  IconBolt,
  IconCalendarWeek,
  IconChartBar,
  IconHeartRateMonitor,
  IconTrendingUp,
} from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { activitiesQuery, fitnessQuery, webUIConfigQuery } from "../../api/queries";
import { formatCalendarDay } from "../../components/chart/TimeFrame";
import { FigureStrip, StripFigure } from "../../components/FigureStrip";
import { PageShell } from "../../components/Layout";
import { Segmented } from "../../components/Segmented";
import { Skeleton } from "../../components/ui/skeleton";
import { DecouplingPanel, DecouplingSummary, decouplingRides } from "./DecouplingPanel";
import { FitnessSection } from "./FitnessSection";
import {
  bandOf,
  FORM_BANDS,
  formPercent,
  RAMP_BANDS,
  type Reading,
  reading,
  type Scale,
  scaleOutlook,
  signed,
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

/** How many days of rest bring form into the fresh band, as the note under form says it. */
function restNote(today: Reading, restForm: readonly number[]): string {
  if (today.formPercent >= 5) {
    return bandOf(FORM_BANDS, today.formPercent).advice;
  }
  const days = restForm.findIndex((percent) => percent >= 5);
  if (days < 0) {
    return "not fresh within three weeks of rest";
  }

  return `fresh after ${days + 1} ${days === 0 ? "day" : "days"} of rest`;
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
      <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-5">
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
              <Segmented
                label="Range"
                size="sm"
                items={RANGES.map(({ value, label }) => ({ key: value, label }))}
                value={range}
                onChange={setRange}
              />
              <Segmented
                label="Scale"
                size="sm"
                items={SCALES.map(({ value, label }) => ({ key: value, label }))}
                value={scale}
                onChange={setScale}
              />
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
            <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
              <div className="flex min-w-0 flex-col gap-5">
                <FitnessSection
                  icon={<IconTrendingUp size={18} stroke={1.8} />}
                  title="Fitness, form and load"
                >
                  <SeasonChart
                    readings={readings}
                    outlook={outlook}
                    scaleName={scaleInfo?.name ?? ""}
                  />
                </FitnessSection>
                {data.powerCurve && data.powerCurve.length > 0 ? (
                  <FitnessSection icon={<IconBolt size={18} stroke={1.8} />} title="Power duration">
                    <PowerDuration
                      current={data.powerCurve}
                      previous={data.powerCurvePrevious}
                      previousName={`the ${selected?.label ?? "range"} before`}
                    />
                    <p className="text-[var(--ink-2)] text-xs">
                      The best each duration reached, from measured power alone. A duration no ride
                      was long enough for carries no point.
                    </p>
                  </FitnessSection>
                ) : null}
              </div>
              <div className="flex min-w-0 flex-col gap-5">
                {data.weeks.length > 0 ? (
                  <FitnessSection
                    icon={<IconChartBar size={18} stroke={1.8} />}
                    title="Time in zone"
                  >
                    <ZonePanel weeks={data.weeks} dates={dates} />
                  </FitnessSection>
                ) : null}
                {activities.isError ? (
                  // An outage must not read as a season with nothing in it.
                  <p className="text-sm text-[var(--alert)]" role="alert">
                    The service did not say what has been ridden, so the season's decoupling is not
                    drawn.
                  </p>
                ) : rides.some((ride) => ride.date >= (dates[0] ?? "")) ? (
                  <FitnessSection
                    icon={<IconHeartRateMonitor size={18} stroke={1.8} />}
                    title="Decoupling"
                    aside={<DecouplingSummary rides={rides} dates={dates} />}
                  >
                    <DecouplingPanel rides={rides} dates={dates} />
                  </FitnessSection>
                ) : null}
              </div>
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
    <FigureStrip>
      <StripFigure
        icon={<IconTrendingUp size={18} stroke={1.8} />}
        label="Fitness"
        value={today.fitness.toFixed(0)}
        chip={signed(today.fitness - start.fitness)}
        tone={today.fitness > start.fitness ? "var(--good)" : "var(--ink-2)"}
        note={`since ${formatCalendarDay(start.date)} · peak ${peak.fitness.toFixed(0)}`}
      />
      {outlook ? <Ramp ramp={outlook.rampPerWeek} fitness={today.fitness} /> : null}
      {outlook ? (
        <StripFigure
          icon={<IconCalendarWeek size={18} stroke={1.8} />}
          label="Next 7 days, to keep building"
          value={`${Math.round(outlook.weekLoadLow)}–${Math.round(outlook.weekLoadHigh)}`}
          unit={unit}
          note={`raises fitness 3–8% · last 4 weeks averaged ${Math.round(outlook.habitualDailyLoad * 7)} a week`}
        />
      ) : null}
      <StripFigure
        icon={<IconHeartRateMonitor size={18} stroke={1.8} />}
        label="Form"
        value={`${signed(today.formPercent)}%`}
        unit="of fitness"
        chip={formBand.name}
        tone={formBand.colour}
        note={outlook ? restNote(today, restForm) : formBand.advice}
      />
    </FigureStrip>
  );
}

function Ramp({ ramp, fitness }: { ramp: number; fitness: number }) {
  const before = fitness - ramp;
  const percent = before > 1 ? (ramp / before) * 100 : 0;
  const band = bandOf(RAMP_BANDS, percent);

  return (
    <StripFigure
      icon={<IconActivityHeartbeat size={18} stroke={1.8} />}
      label="Ramp rate"
      value={signed(ramp, 1)}
      unit="a week"
      chip={band.name}
      tone={band.colour}
      note={`${signed(percent)}% of fitness`}
    />
  );
}
