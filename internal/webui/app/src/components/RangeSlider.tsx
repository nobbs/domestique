import { Slider } from "@base-ui/react/slider";
import type { NumericRange } from "../lib/filters";
import { histogram } from "../lib/histogram";

const BIN_COUNT = 24;

function clamp(value: number, low: number, high: number): number {
  return Math.min(Math.max(value, low), high);
}

export interface RangeSliderProps {
  legend: string;
  /** The slider's domain in the stored unit. A thumb parked at either edge means unbounded. */
  min: number;
  max: number;
  step: number;
  range: NumericRange;
  onChange: (next: NumericRange) => void;
  /** A stored value as the legend and the thumbs read it out. */
  format: (stored: number) => string;
  /** Every route's value of this measure, drawn over the track so a bound shows what it cuts off. */
  values: number[];
}

/**
 * Two thumbs on one track. The ends read as "no bound", so an untouched
 * slider filters nothing and a route past the domain's edge still passes.
 */
export function RangeSlider({
  legend,
  min,
  max,
  step,
  range,
  onChange,
  format,
  values,
}: RangeSliderProps) {
  // Clamped to the slider's own domain and ordered low-to-high: a stored
  // range this component did not produce must never hand Base UI a value
  // outside `[min, max]` or with the thumbs crossed.
  const lo = clamp(range.min ?? min, min, max);
  const hi = clamp(range.max ?? max, lo, max);
  const isSet = range.min !== null || range.max !== null;
  const bins = histogram(values, min, max, BIN_COUNT);
  const peak = Math.max(1, ...bins);
  const binWidth = (max - min) / BIN_COUNT;

  return (
    <div className="grid gap-1">
      <div className="flex items-baseline justify-between text-sm">
        <span className="font-semibold">{legend}</span>
        <span className={isSet ? "tabular-nums" : "text-[var(--ink-2)]"}>
          {isSet ? `${format(lo)} – ${format(hi)}` : "any"}
        </span>
      </div>
      <div className="px-2">
        <div className="flex h-7 items-end gap-px" aria-hidden="true">
          {bins.map((count, index) => {
            const start = min + index * binWidth;
            const inside = start + binWidth > lo && start < hi;
            return (
              <div
                // biome-ignore lint/suspicious/noArrayIndexKey: bins are positional
                key={index}
                className={inside ? "flex-1 bg-[var(--accent)]/60" : "flex-1 bg-[var(--rule)]"}
                style={{ height: `${Math.max(count === 0 ? 0 : 6, (count / peak) * 100)}%` }}
              />
            );
          })}
        </div>
        <Slider.Root
          min={min}
          max={max}
          step={step}
          value={[lo, hi]}
          onValueChange={(value) => {
            const [nextLo = min, nextHi = max] = value;
            onChange({ min: nextLo <= min ? null : nextLo, max: nextHi >= max ? null : nextHi });
          }}
          className="relative flex w-full touch-none items-center select-none"
        >
          <Slider.Control className="flex w-full items-center py-1">
            <Slider.Track className="relative h-1 w-full grow rounded-full bg-[var(--rule)]">
              <Slider.Indicator
                className={isSet ? "absolute h-full rounded-full bg-[var(--accent)]" : "hidden"}
              />
              {(["Min", "Max"] as const).map((side) => (
                <Slider.Thumb
                  key={side}
                  aria-label={`${legend} ${side.toLowerCase()}`}
                  getAriaValueText={(_formatted, value) => format(value)}
                  className="block size-4 shrink-0 rounded-full border border-[var(--accent)] bg-[var(--panel)] shadow-sm outline-none focus-visible:ring-3 focus-visible:ring-[var(--accent)]/40"
                />
              ))}
            </Slider.Track>
          </Slider.Control>
        </Slider.Root>
      </div>
      <div className="flex justify-between px-2 text-[11px] text-[var(--ink-2)] tabular-nums">
        <span>{format(min)}</span>
        <span>{format(max)}</span>
      </div>
    </div>
  );
}
