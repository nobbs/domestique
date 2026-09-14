import type { ReactNode } from "react";

/** One titled panel of the page, with whatever qualifies its title beside it. */
export function FitnessSection({
  title,
  aside,
  children,
}: {
  title: string;
  aside?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section
      aria-label={title}
      className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="font-semibold text-base">{title}</h2>
        {aside}
      </div>
      {children}
    </section>
  );
}

/** One headline figure: its name, the number, and a line on what it means. */
export function FitnessStat({
  label,
  value,
  unit,
  note,
  tone,
}: {
  label: string;
  value: string;
  unit?: string;
  note?: string;
  /** A status colour marking the note, which always names the status in words too. */
  tone?: string;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="flex flex-col gap-0.5 rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5"
    >
      <span className="text-[var(--ink-2)] text-xs">{label}</span>
      <span className="font-semibold text-2xl tabular-nums tracking-tight">
        {value}
        {unit ? <span className="ml-1 font-normal text-[var(--ink-2)] text-sm">{unit}</span> : null}
      </span>
      {note ? (
        <span className="flex items-center gap-1.5 text-[var(--ink-2)] text-xs">
          {tone ? (
            <span
              aria-hidden="true"
              className="size-2 shrink-0 rounded-full"
              style={{ background: tone }}
            />
          ) : null}
          {note}
        </span>
      ) : null}
    </div>
  );
}
