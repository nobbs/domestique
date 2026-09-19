import {
  IconCheck,
  IconInfoCircle,
  IconPlayerPause,
  IconPlayerPlay,
  IconRoute,
  IconScissors,
  IconSeedling,
  IconX,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import { Button } from "../../components/Button";
import { cn } from "../../lib/utils";

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
      <p role="status" className="text-xs tabular-nums">
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

const STOPPED_SHORT =
  "Tracing stopped before the plan fully follows the copied route; the dashed line shows where they differ.";

/** A trace the waypoint cap or its round limit stopped before the plan follows the copied route. */
export function TraceStoppedShort({ onDismiss }: { onDismiss: () => void }) {
  return (
    <div
      role="status"
      className="-translate-x-1/2 absolute top-3 left-1/2 z-30 flex items-center gap-1.5 rounded-full bg-[var(--panel)] py-1 pr-1 pl-3 text-[var(--hold)] text-xs shadow-[var(--shadow)]"
    >
      <span className="sr-only">{STOPPED_SHORT}</span>
      <span aria-hidden="true">Stopped short of the copied route</span>
      <span title={STOPPED_SHORT} aria-hidden="true" className="cursor-help">
        <IconInfoCircle size={13} />
      </span>
      <Button
        variant="ghost"
        icon={<IconX size={13} />}
        aria-label="Dismiss"
        title="Dismiss"
        onClick={onDismiss}
      />
    </div>
  );
}
