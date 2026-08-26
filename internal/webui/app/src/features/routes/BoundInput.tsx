import { useId, useState } from "react";
import { Input } from "../../components/ui/input";

/** One field's text, `null` for an empty — and so unbounded — side. */
function parseBound(value: string): number | null {
  if (value.trim() === "") {
    return null;
  }
  const parsed = Number(value);

  return Number.isFinite(parsed) ? parsed : null;
}

function displayOf(stored: number | null, toDisplay: (stored: number) => number): string {
  return stored === null ? "" : String(toDisplay(stored));
}

export interface BoundInputProps {
  label: string;
  /** The bound in its stored unit — metres, percent — not what the field shows. */
  stored: number | null;
  onChange: (stored: number | null) => void;
  toDisplay: (stored: number) => number;
  toStored: (display: number) => number;
}

/**
 * One bound, typed as free text rather than driven straight from the stored
 * number.
 *
 * A controlled field whose value is `toDisplay(stored)` on every keystroke
 * fights whatever was just typed: "1." parses to the same number as "1", so
 * a value re-derived from the parsed number drops the point before a second
 * digit can follow it, and typing a fraction becomes impossible. The text is
 * kept here instead, and is resynced from `stored` only when it no longer
 * matches this field's own last edit — the signal that the change came from
 * outside, such as Clear filters, rather than from what was just typed.
 *
 * `type="text"` rather than `type="number"`, deliberately: a number input's
 * own `.value` reports empty for "1." too, per the HTML value-sanitisation
 * algorithm — a partial decimal is not yet a complete floating-point number —
 * so the browser itself would hand back the empty string this component is
 * trying to stop reading. `inputMode="decimal"` still asks a touch keyboard
 * for digits and a point.
 */
export function BoundInput({ label, stored, onChange, toDisplay, toStored }: BoundInputProps) {
  const id = useId();
  const [text, setText] = useState(() => displayOf(stored, toDisplay));
  const [lastStored, setLastStored] = useState(stored);

  if (stored !== lastStored) {
    setLastStored(stored);
    setText(displayOf(stored, toDisplay));
  }

  return (
    <label className="grid gap-1 text-xs text-[var(--ink-2)]" htmlFor={id}>
      <span>{label}</span>
      <Input
        id={id}
        className="bg-[var(--panel)]"
        type="text"
        inputMode="decimal"
        value={text}
        onChange={(event) => {
          const raw = event.target.value;
          setText(raw);
          const parsed = parseBound(raw);
          const next = parsed === null ? null : toStored(parsed);
          setLastStored(next);
          onChange(next);
        }}
      />
    </label>
  );
}
