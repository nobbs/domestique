import type { ReactNode } from "react";
import { PanelMark } from "./PanelHeading";

/** The headline figures, one card split by hairlines once they fit a row. */
export function FigureStrip({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)] sm:grid-cols-2 lg:grid-cols-4 lg:gap-0 lg:divide-x lg:divide-[var(--rule)]">
      {children}
    </div>
  );
}

/** One headline figure: its mark, name, the number, a verdict chip and a line on what it means. */
export function StripFigure({
  icon,
  label,
  value,
  unit,
  chip,
  tone = "var(--ink-2)",
  note,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  unit?: string;
  chip?: string;
  /** The chip's colour; the chip always names the status in words too. */
  tone?: string;
  note?: ReactNode;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="flex min-w-0 items-center gap-3 lg:px-4 lg:first:pl-0 lg:last:pr-0"
    >
      <PanelMark>{icon}</PanelMark>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="text-[var(--ink-2)] text-xs">{label}</span>
        <span className="flex items-baseline justify-between gap-2">
          <span className="font-semibold text-xl tabular-nums tracking-tight">
            {value}
            {unit ? (
              <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">{unit}</span>
            ) : null}
          </span>
          {chip ? (
            <span
              className="whitespace-nowrap rounded-md px-1.5 py-0.5 font-medium text-xs"
              style={{ background: `color-mix(in oklab, ${tone} 14%, transparent)`, color: tone }}
            >
              {chip}
            </span>
          ) : null}
        </span>
        {note ? <span className="truncate text-[var(--ink-2)] text-xs">{note}</span> : null}
      </div>
    </div>
  );
}
