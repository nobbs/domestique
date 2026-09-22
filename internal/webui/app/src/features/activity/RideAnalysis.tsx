/**
 * What a language model made of one ride: a card per field for a structured
 * document, or its plain text for an analysis written before that document
 * existed, or on a deployment that never had a token. Absent until the ride
 * has been analysed. An admin may ask again about any derived ride while
 * analysis is on.
 */

import {
  IconBolt,
  IconGauge,
  IconHeart,
  IconInfoCircle,
  IconMountain,
  IconRefresh,
  IconRotateClockwise2,
  IconSparkles,
  IconWind,
} from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import type { ComponentType, ReactNode } from "react";
import type { ActivityAnalysisDocumentRideType } from "../../api/generated";
import { useReanalyseActivity } from "../../api/generated";
import { fitnessQuery, tasksQuery, webUIConfigQuery } from "../../api/queries";
import { TASKS } from "../../api/tasks";
import type { Activity, ActivityAnalysisDocument } from "../../api/types";
import { Button } from "../../components/Button";
import { PanelHeading } from "../../components/PanelHeading";
import { formatTimestamp } from "../../lib/format";
import { useEffectiveAdmin } from "../../lib/identity";
import { calendarDay } from "../fitness/DecouplingPanel";
import { signed } from "../fitness/form";

/**
 * Splits at the first `. `/`! `/`? ` that is followed by a capital letter, for
 * the lead sentence's emphasis; the whole text is the first sentence when no
 * such boundary exists (a lone sentence, or every break followed by a digit
 * or lowercase word such as a decimal).
 */
export function firstSentence(text: string): { first: string; rest: string } {
  const match = /[.!?] (?=[A-Z])/.exec(text);
  if (!match) {
    return { first: text, rest: "" };
  }
  const end = match.index + 1;

  return { first: text.slice(0, end), rest: text.slice(end) };
}

/** The calendar day shifted by `delta` days, both as `YYYY-MM-DD`. */
function shiftDay(date: string, delta: number): string {
  const at = new Date(`${date}T00:00:00Z`);
  at.setUTCDate(at.getUTCDate() + delta);

  return at.toISOString().slice(0, 10);
}

/**
 * The window the load card's form figure is read from, as the instants the
 * endpoint takes: a day either side of the ride's, so the ride's own day is
 * inside it whatever zone the service cuts days in.
 */
export function fitnessWindowFor(rideDay: string): { from: string; to: string } {
  return { from: `${shiftDay(rideDay, -1)}T00:00:00Z`, to: `${shiftDay(rideDay, 2)}T00:00:00Z` };
}

export function RideAnalysis({ ride }: { ride: Activity | undefined }) {
  const admin = useEffectiveAdmin();
  const tasks = useQuery({ ...tasksQuery(), enabled: admin });
  const config = useQuery(webUIConfigQuery());
  const reanalyse = useReanalyseActivity();
  const analysis = ride?.analysis;
  const doc = analysis?.document;
  const canAsk =
    admin &&
    ride?.metrics !== undefined &&
    (tasks.data?.tasks.some((task) => task.name === TASKS.activityReanalyse) ?? false);
  if (!ride || (!analysis && !canAsk)) {
    return null;
  }

  return (
    <section
      className="flex flex-col gap-2 rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)]"
      aria-label="Analysis"
    >
      <PanelHeading
        icon={<IconSparkles size={18} stroke={1.8} />}
        title="Analysis"
        aside={
          doc || canAsk ? (
            <div className="flex items-center gap-2">
              {doc ? <RideTypeChip type={doc.rideType} /> : null}
              {canAsk ? (
                <Button
                  variant="outline"
                  icon={<IconRefresh size={18} stroke={1.8} />}
                  aria-label={analysis ? "Analyse again" : "Analyse"}
                  title={analysis ? "Analyse again" : "Analyse"}
                  disabled={reanalyse.isPending || reanalyse.isSuccess}
                  onClick={() => reanalyse.mutate({ activityId: ride.id })}
                />
              ) : null}
            </div>
          ) : undefined
        }
      />
      {analysis ? (
        <>
          {doc ? (
            <RideAnalysisDocument
              analysis={doc}
              ride={ride}
              zone={config.data?.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone}
            />
          ) : (
            // The model is asked for plain paragraphs; any markup it returns shows as typed.
            <p className="whitespace-pre-line text-sm leading-relaxed">{analysis.text}</p>
          )}
          <p className="text-[var(--ink-2)] text-xs">
            {analysis.model} · {formatTimestamp(analysis.analysedAt)}
          </p>
        </>
      ) : null}
      {reanalyse.isSuccess ? (
        <p className="text-[var(--ink-2)] text-xs" role="status">
          Asked. Reload the page in a minute to see the new analysis; a failed request leaves this
          one as it is.
        </p>
      ) : null}
      {reanalyse.isError ? (
        <p className="text-[var(--ink-2)] text-xs" role="status">
          The service did not take the request.
        </p>
      ) : null}
    </section>
  );
}

/** Background/ink pair per ride type, every hue a token so each reads in both themes. */
export const RIDE_TYPE_CHIP: Record<ActivityAnalysisDocumentRideType, string> = {
  recovery: "bg-[color-mix(in_srgb,var(--good)_15%,transparent)] text-[var(--good)]",
  endurance: "bg-[color-mix(in_srgb,var(--accent)_15%,transparent)] text-[var(--accent)]",
  tempo: "bg-[color-mix(in_srgb,var(--hold)_15%,transparent)] text-[var(--hold)]",
  intervals: "bg-[color-mix(in_srgb,var(--alert)_15%,transparent)] text-[var(--alert)]",
  threshold:
    "bg-[color-mix(in_srgb,var(--ride-threshold)_15%,transparent)] text-[var(--ride-threshold)]",
  race: "bg-[color-mix(in_srgb,var(--ride-race)_15%,transparent)] text-[var(--ride-race)]",
  mixed: "bg-[color-mix(in_srgb,var(--ride-mixed)_15%,transparent)] text-[var(--ride-mixed)]",
  commute: "bg-[color-mix(in_srgb,var(--ride-commute)_15%,transparent)] text-[var(--ride-commute)]",
};

function RideTypeChip({ type }: { type: ActivityAnalysisDocumentRideType }) {
  return (
    <span
      className={`inline-flex h-[22px] w-fit shrink-0 items-center gap-1.5 rounded-full px-2.5 font-semibold text-xs ${RIDE_TYPE_CHIP[type]}`}
    >
      <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-current" />
      {type}
    </span>
  );
}

/** The card's own background, label and body — a uniform frame for the three findings. */
function Card({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5 rounded-xl bg-[var(--base)] p-3.5">
      <span className="font-semibold text-[var(--ink-2)] text-xs uppercase tracking-wide">
        {label}
      </span>
      {children}
    </div>
  );
}

const GOOD_PILL = "bg-[color-mix(in_srgb,var(--good)_15%,transparent)] text-[var(--good)]";
const HOLD_PILL = "bg-[color-mix(in_srgb,var(--hold)_15%,transparent)] text-[var(--hold)]";

function Pill({ tone, children }: { tone: "good" | "hold"; children: ReactNode }) {
  return (
    <span
      className={`inline-flex h-5 w-fit shrink-0 items-center rounded-full px-2 font-semibold text-xs ${tone === "good" ? GOOD_PILL : HOLD_PILL}`}
    >
      {children}
    </span>
  );
}

function Figure({ value, label }: { value: string; label: string }) {
  return (
    <div className="flex flex-col">
      <span className="font-semibold text-xl leading-tight">{value}</span>
      <span className="text-[11px] text-[var(--ink-2)]">{label}</span>
    </div>
  );
}

function NextSessionCard({
  nextSession,
}: {
  nextSession: ActivityAnalysisDocument["nextSession"];
}) {
  const noRest = nextSession.suggestedRestDays === 0;

  return (
    <Card label="Next session">
      <p className="text-[13px] leading-relaxed">{nextSession.advice}</p>
      <Pill tone={noRest ? "good" : "hold"}>
        {noRest
          ? "no rest needed"
          : `rest ${nextSession.suggestedRestDays} ${nextSession.suggestedRestDays === 1 ? "day" : "days"}`}
      </Pill>
    </Card>
  );
}

/** The ride's own TSS, TRIMP and form beside the model's read of what they add up to. */
function LoadCard({
  ride,
  loadEffect,
  form,
}: {
  ride: Activity;
  loadEffect: string;
  form: number | undefined;
}) {
  const metrics = ride.metrics;
  const tss = metrics?.powerTss ?? metrics?.heartRateTss;
  // heartRateTss is a different scale than power TSS; RideFigures.tsx names it "hrTSS" for the same reason.
  const tssLabel =
    metrics?.powerTss === undefined && metrics?.heartRateTss !== undefined ? "hrTSS" : "TSS";

  return (
    <Card label="Load">
      <div className="flex gap-3.5">
        <Figure value={tss === undefined ? "–" : tss.toFixed(0)} label={tssLabel} />
        <Figure
          value={metrics?.trimp === undefined ? "–" : metrics.trimp.toFixed(0)}
          label="TRIMP"
        />
        <Figure value={form === undefined ? "–" : signed(form)} label="form" />
      </div>
      <p className="text-[12px] text-[var(--ink-2)]">{loadEffect}</p>
    </Card>
  );
}

function WatchOutCard({ concerns }: { concerns: string[] }) {
  return (
    <Card label="Watch out">
      {concerns.length > 0 ? (
        <ul className="list-disc space-y-1 pl-4 text-[13px] leading-relaxed">
          {concerns.map((concern) => (
            <li key={concern}>{concern}</li>
          ))}
        </ul>
      ) : (
        <p className="text-[13px] text-[var(--ink-2)]">Nothing to flag</p>
      )}
      {concerns.length > 0 ? (
        <Pill tone="hold">
          {concerns.length} {concerns.length === 1 ? "concern" : "concerns"}
        </Pill>
      ) : null}
    </Card>
  );
}

function RideAnalysisDocument({
  analysis,
  ride,
  zone,
}: {
  analysis: ActivityAnalysisDocument;
  ride: Activity;
  zone: string;
}) {
  const rideDay = calendarDay(ride.startedAt, zone);
  const fitness = useQuery(fitnessQuery(fitnessWindowFor(rideDay)));
  const day = fitness.data?.days.find((candidate) => candidate.date === rideDay);
  const hasTss = ride.metrics?.powerTss !== undefined || ride.metrics?.heartRateTss !== undefined;
  const form = day === undefined ? undefined : hasTss ? day.tssForm : day.trimpForm;
  const { first, rest } = firstSentence(analysis.summary);

  return (
    <>
      <p className="font-semibold text-lg leading-snug">{analysis.headline}</p>
      <p className="whitespace-pre-line text-base leading-relaxed">
        <span className="font-medium">{first}</span>
        {rest}
      </p>
      <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-3">
        <NextSessionCard nextSession={analysis.nextSession} />
        <LoadCard ride={ride} loadEffect={analysis.loadEffect} form={form} />
        <WatchOutCard concerns={analysis.concerns} />
      </div>
      {analysis.highlights.length > 0 ? (
        <div className="flex flex-col gap-1.5">
          <span className="font-semibold text-[var(--ink-2)] text-xs uppercase tracking-wide">
            Highlights
          </span>
          <div className="flex flex-col">
            {analysis.highlights.map((highlight) => {
              const Icon = highlightIcon(highlight);
              return (
                <div
                  key={highlight}
                  className="flex items-center gap-3 border-[var(--rule)] border-t py-2"
                >
                  <span
                    aria-hidden="true"
                    className="grid size-7 shrink-0 place-items-center rounded-full bg-[var(--base)]"
                  >
                    <Icon size={14} />
                  </span>
                  <span className="text-[14px] leading-relaxed">{highlight}</span>
                </div>
              );
            })}
          </div>
        </div>
      ) : null}
      {analysis.dataGaps.length > 0 ? (
        <div className="flex items-center gap-2 rounded-lg bg-[var(--base)] px-2.5 py-2 text-[var(--ink-2)] text-xs">
          <IconInfoCircle size={13} aria-hidden="true" />
          <span>Not measured: {analysis.dataGaps.join(", ")}</span>
        </div>
      ) : null}
    </>
  );
}

type Mark = ComponentType<{ size?: number; stroke?: number }>;

const HIGHLIGHT_ICONS: [RegExp, Mark][] = [
  [/\bW\b|watt|power|target/i, IconBolt],
  [/bpm|heart|hr\b|zone/i, IconHeart],
  [/rpm|cadence/i, IconRotateClockwise2],
  [/km\/h|speed|pace|split/i, IconGauge],
  [/climb|ascent|gradient|grade|\bm\b/i, IconMountain],
  [/wind|tailwind|headwind|rain|drizzle|°C|temperature/i, IconWind],
];

/** The glyph a highlight line earns, by the first pattern it matches; `IconSparkles` otherwise. */
export function highlightIcon(text: string): Mark {
  for (const [pattern, icon] of HIGHLIGHT_ICONS) {
    if (pattern.test(text)) {
      return icon;
    }
  }

  return IconSparkles;
}
