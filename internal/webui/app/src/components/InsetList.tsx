/**
 * A card's list as one muted block, its rows split by the card's own colour,
 * each led by a mark that says how the thing it names is doing.
 */

import { IconAlertTriangle, IconCheck, IconClockPause, IconMinus } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** How the thing a row names is doing; `quiet` is nothing to report either way. */
export type RowTone = "good" | "hold" | "alert" | "quiet";

const COLOUR: Record<RowTone, string> = {
  good: "var(--good)",
  hold: "var(--hold)",
  alert: "var(--alert)",
  quiet: "var(--ink-2)",
};

const GLYPH: Record<RowTone, typeof IconCheck> = {
  good: IconCheck,
  hold: IconClockPause,
  alert: IconAlertTriangle,
  quiet: IconMinus,
};

const tint = (tone: RowTone, share: number) =>
  `color-mix(in oklab, ${COLOUR[tone]} ${share}%, transparent)`;

export function InsetList({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <ul
      className={cn(
        "flex flex-col overflow-hidden rounded-xl bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]",
        className,
      )}
    >
      {children}
    </ul>
  );
}

/**
 * One row: the mark, the name with one muted line under it, whatever acts on it
 * at the end, and a note beneath when something needs the reader.
 */
export function InsetRow({
  tone,
  /** Says in words what the mark says in colour, for whoever cannot see it. */
  toneLabel,
  title,
  detail,
  actions,
  note,
  ...attributes
}: {
  tone: RowTone;
  toneLabel?: string;
  title: ReactNode;
  /** One muted line; the parts of an array are set apart by a middle dot, empty ones skipped. */
  detail?: ReactNode | readonly ReactNode[];
  actions?: ReactNode;
  note?: ReactNode;
} & Record<`data-${string}`, string | undefined>) {
  const Glyph = GLYPH[tone];

  return (
    <li
      className="flex flex-col gap-2 border-[var(--panel)] border-b-2 px-3.5 py-3 text-sm last:border-b-0"
      {...attributes}
    >
      <div className="flex items-center gap-3">
        <span
          className="grid size-8 shrink-0 place-items-center rounded-[9px]"
          style={{ background: tint(tone, 18), color: COLOUR[tone] }}
          title={toneLabel}
          {...(toneLabel ? { role: "img", "aria-label": toneLabel } : { "aria-hidden": true })}
        >
          <Glyph size={16} stroke={2} aria-hidden="true" />
        </span>
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="truncate font-semibold">{title}</span>
          <Detail parts={detail} />
        </div>
        {actions ? (
          <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>
        ) : null}
      </div>
      {note}
    </li>
  );
}

function Detail({ parts }: { parts: ReactNode | readonly ReactNode[] }) {
  const shown = (Array.isArray(parts) ? parts : [parts]).filter(
    (part) => part !== null && part !== undefined && part !== false && part !== "",
  );
  if (shown.length === 0) {
    return null;
  }

  return (
    <span className="text-[var(--ink-2)] text-xs">
      {shown.map((part, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: the parts are fixed and never reorder
        <span key={index}>
          {index > 0 ? " · " : null}
          <span>{part}</span>
        </span>
      ))}
    </span>
  );
}

/** What a row needs from the reader, tinted in its tone and set under its name. */
export function RowNote({
  tone,
  children,
  ...attributes
}: { tone: RowTone; children: ReactNode } & Record<`data-${string}`, string | undefined>) {
  return (
    <span
      className="ml-11 rounded-lg px-2.5 py-1.5 text-xs"
      style={{ background: tint(tone, 10), color: COLOUR[tone] }}
      {...attributes}
    >
      {children}
    </span>
  );
}
