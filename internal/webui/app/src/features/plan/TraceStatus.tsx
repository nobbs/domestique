import {
  IconCheck,
  IconPlayerPause,
  IconPlayerPlay,
  IconRoute,
  IconScissors,
  IconSeedling,
  IconX,
} from "@tabler/icons-react";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { Button } from "../../components/Button";
import { usePrefersReducedMotion } from "../../lib/mediaQuery";
import { cn } from "../../lib/utils";
import type { TraceSummary } from "./planner";

type StepStatus = "done" | "current" | "pending";

const STEPS = [
  { key: "seed", label: "Seed", icon: <IconSeedling size={12} aria-hidden="true" /> },
  { key: "add", label: "Match", icon: <IconRoute size={12} aria-hidden="true" /> },
  { key: "prune", label: "Trim", icon: <IconScissors size={12} aria-hidden="true" /> },
] as const;

/** Why a trace is holding its place: the admin paused it, or the engine asked for a retry. */
export type TracePause = "user" | "busy" | null;

function StepCircle({
  status,
  label,
  icon,
  pause,
}: {
  status: StepStatus;
  label: string;
  icon: ReactNode;
  pause: TracePause;
}) {
  const paused = status === "current" ? pause : null;

  return (
    <span
      title={label}
      className={cn(
        "relative grid size-6 place-items-center rounded-full border",
        status === "done" &&
          "border-[var(--good)] bg-[color-mix(in_oklab,var(--good)_16%,transparent)] text-[var(--good)]",
        status === "current" && paused === null && "border-[var(--accent)] text-[var(--accent)]",
        paused === "user" && "border-[var(--ink-2)] text-[var(--ink-2)]",
        paused === "busy" && "border-[var(--alert)] text-[var(--alert)]",
        status === "pending" && "border-[var(--rule)] text-[var(--ink-2)]",
      )}
    >
      {icon}
      {status === "done" ? (
        <IconCheck
          size={9}
          aria-hidden="true"
          className="absolute right-[-3px] bottom-[-3px] rounded-full bg-[var(--panel)] text-[var(--good)]"
        />
      ) : null}
      {paused ? (
        <IconPlayerPause
          size={8}
          aria-hidden="true"
          className={cn(
            "absolute right-[-3px] bottom-[-3px] rounded-full bg-[var(--panel)]",
            paused === "busy" ? "text-[var(--alert)]" : "text-[var(--ink-2)]",
          )}
        />
      ) : null}
      {status === "current" && paused === null ? (
        <span className="absolute inset-[-3px] rounded-full border border-[var(--accent)] motion-safe:animate-ping motion-reduce:hidden" />
      ) : null}
    </span>
  );
}

/** A running or paused trace: its steps, the waypoint count, and the controls that hold or end it. */
export function TraceStatus({
  phase,
  waypoints,
  pause,
  onPause,
  onResume,
  onCancel,
}: {
  phase: "add" | "prune";
  waypoints: number;
  pause: TracePause;
  onPause: () => void;
  onResume: () => void;
  onCancel: () => void;
}) {
  const statusOf = (key: (typeof STEPS)[number]["key"]): StepStatus => {
    const order = { seed: 0, add: 1, prune: 2 } as const;
    return order[key] < order[phase] ? "done" : key === phase ? "current" : "pending";
  };
  const sentence =
    pause === "busy"
      ? "Tracing paused: the routing engine is busy."
      : pause === "user"
        ? `Tracing paused with ${waypoints} waypoints.`
        : `Tracing the copied route with ${waypoints} waypoints…`;

  return (
    <div className="-translate-x-1/2 absolute top-3 left-1/2 z-30 flex items-center gap-3 rounded-full bg-[var(--panel)] py-1.5 pr-1.5 pl-3 shadow-[var(--shadow)]">
      <div className="flex items-center gap-0.5" aria-hidden="true">
        {STEPS.map((step, index) => (
          <div key={step.key} className="flex items-center gap-0.5">
            {index > 0 ? <span className="h-px w-3 bg-[var(--rule)]" /> : null}
            <StepCircle
              status={statusOf(step.key)}
              label={step.label}
              icon={step.icon}
              pause={pause}
            />
          </div>
        ))}
      </div>
      <p role="status" className="whitespace-nowrap text-xs tabular-nums">
        <span className="sr-only">{sentence}</span>
        <span aria-hidden="true" className={cn(pause === "busy" && "text-[var(--alert)]")}>
          {pause === "busy"
            ? "Engine busy"
            : pause === "user"
              ? `Paused · ${waypoints} wp`
              : `${waypoints} wp`}
        </span>
      </p>
      <div className="flex items-center">
        {pause ? (
          <Button
            variant="ghost"
            icon={<IconPlayerPlay size={14} />}
            aria-label="Resume tracing"
            title="Resume tracing"
            onClick={onResume}
          />
        ) : (
          <Button
            variant="ghost"
            icon={<IconPlayerPause size={14} />}
            aria-label="Pause tracing"
            title="Pause tracing, to edit the plan"
            onClick={onPause}
          />
        )}
        <Button
          variant="ghost"
          icon={<IconX size={14} />}
          aria-label="Cancel tracing"
          title="Cancel tracing, keeping these waypoints"
          onClick={onCancel}
        />
      </div>
    </div>
  );
}

/** Three checked Seed·Match·Trim circles: the resting shape the running steps settle into. */
function DoneSteps() {
  return (
    <div className="flex items-center gap-0.5" aria-hidden="true">
      {STEPS.map((step, index) => (
        <div key={step.key} className="flex items-center gap-0.5">
          {index > 0 ? <span className="h-px w-3 bg-[var(--rule)]" /> : null}
          <StepCircle status="done" label={step.label} icon={step.icon} pause={null} />
        </div>
      ))}
    </div>
  );
}

function formatPercent(share: number): string {
  return `${(share * 100).toFixed(1)}%`;
}

function formatKm(km: number): string {
  return `${km.toFixed(1)} km`;
}

/** seed → peak → final waypoints, rounds, time, % followed, distance, and strayed stretches when any. */
function TraceStats({ summary }: { summary: TraceSummary }) {
  const rows: [string, string][] = [
    [
      "Waypoints",
      `${summary.waypoints.seed} → ${summary.waypoints.peak} → ${summary.waypoints.final}`,
    ],
    ["Rounds", `${summary.rounds}`],
    ["Time", `${summary.seconds} s`],
    ["Followed", formatPercent(summary.followedShare)],
    ["Distance", `${formatKm(summary.planKm)} of ${formatKm(summary.copiedKm)}`],
  ];
  if (summary.strayedStretches > 0) {
    rows.push([
      "Strayed",
      `${summary.strayedStretches} stretch${summary.strayedStretches === 1 ? "" : "es"}`,
    ]);
  }

  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
      {rows.map(([term, value]) => (
        <div key={term} className="contents">
          <dt className="text-[var(--ink-2)]">{term}</dt>
          <dd className="text-right tabular-nums">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function CountdownRing({ fraction }: { fraction: number }) {
  const radius = 8;
  const circumference = 2 * Math.PI * radius;

  return (
    <svg
      width={20}
      height={20}
      viewBox="0 0 20 20"
      aria-hidden="true"
      className="shrink-0"
      data-testid="trace-countdown"
    >
      <circle
        cx={10}
        cy={10}
        r={radius}
        className="fill-none stroke-[var(--rule)]"
        strokeWidth={2}
      />
      <circle
        cx={10}
        cy={10}
        r={radius}
        className="fill-none stroke-[var(--accent)]"
        strokeWidth={2}
        strokeLinecap="round"
        strokeDasharray={circumference}
        strokeDashoffset={circumference * (1 - fraction)}
        transform="rotate(-90 10 10)"
      />
    </svg>
  );
}

/** How long a finished trace's chip stays before it hides itself. */
const FINISHED_HIDE_MS = 10_000;

/**
 * A pausable countdown in milliseconds. `active` toggling off freezes the
 * remaining time rather than resetting it; toggling back on resumes from there.
 */
function useCountdown(totalMs: number, active: boolean): number {
  const [remaining, setRemaining] = useState(totalMs);
  const remainingRef = useRef(totalMs);

  useEffect(() => {
    if (!active) {
      return;
    }
    const start = Date.now();
    const from = remainingRef.current;
    const id = window.setInterval(() => {
      const left = Math.max(0, from - (Date.now() - start));
      remainingRef.current = left;
      setRemaining(left);
      if (left <= 0) {
        window.clearInterval(id);
      }
    }, 100);
    return () => window.clearInterval(id);
  }, [active]);

  return remaining;
}

/** A trace that finished, or that the waypoint cap or round limit stopped short. */
export function TraceFinished({
  summary,
  onDismiss,
}: {
  summary: TraceSummary;
  onDismiss: () => void;
}) {
  const reducedMotion = usePrefersReducedMotion();
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const stoppedShort = summary.outcome === "stoppedShort";
  const autoHide = !stoppedShort;
  const remaining = useCountdown(FINISHED_HIDE_MS, autoHide && !hovered && !focused);

  useEffect(() => {
    if (autoHide && remaining <= 0) {
      onDismiss();
    }
  }, [autoHide, remaining, onDismiss]);

  const sentence = stoppedShort
    ? "Tracing stopped before the plan fully follows the copied route; the red stretches show where they differ."
    : `Tracing finished with ${summary.waypoints.final} waypoints; the plan follows ${(summary.followedShare * 100).toFixed(1)} % of the copied route.`;

  return (
    <div
      className="-translate-x-1/2 absolute top-3 left-1/2 z-30 flex flex-col items-center gap-1.5"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
    >
      <div className="flex items-center gap-3 rounded-full bg-[var(--panel)] py-1.5 pr-1.5 pl-3 shadow-[var(--shadow)]">
        <DoneSteps />
        <p role="status" className="whitespace-nowrap text-xs tabular-nums">
          <span className="sr-only">{sentence}</span>
          <span aria-hidden="true" className={cn(stoppedShort && "text-[var(--hold)]")}>
            {stoppedShort
              ? `Stopped short · ${summary.waypoints.final} wp`
              : `${summary.waypoints.final} wp`}
          </span>
        </p>
        <div className="flex items-center">
          {autoHide ? (
            <span
              className="grid size-8 place-items-center"
              title={`Hides in ${Math.ceil(remaining / 1000)} s`}
            >
              {reducedMotion ? (
                <span className="text-[var(--ink-2)] text-xs tabular-nums">
                  {Math.ceil(remaining / 1000)}s
                </span>
              ) : (
                <CountdownRing fraction={remaining / FINISHED_HIDE_MS} />
              )}
            </span>
          ) : null}
          <Button
            variant="ghost"
            icon={<IconX size={14} />}
            aria-label="Dismiss"
            title="Dismiss"
            onClick={onDismiss}
          />
        </div>
      </div>
      <div className="w-[min(280px,90vw)] rounded-[11px] bg-[var(--panel)] px-3 py-2 shadow-[var(--shadow)]">
        <TraceStats summary={summary} />
      </div>
    </div>
  );
}
