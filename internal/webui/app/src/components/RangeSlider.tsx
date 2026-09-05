import { Slider } from "@base-ui/react/slider";
import type { NumericRange } from "../lib/filters";

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
}

/**
 * Two thumbs on one track, bounded on both sides by a fixed domain rather
 * than by the library: the ends read as "no bound" so an untouched slider
 * filters nothing, and a route past the domain's edge still passes.
 */
export function RangeSlider({ legend, min, max, step, range, onChange, format }: RangeSliderProps) {
  const lo = range.min ?? min;
  const hi = range.max ?? max;

  return (
    <div className="grid gap-2">
      <div className="flex justify-between text-sm">
        <span className="font-semibold">{legend}</span>
        <span className="text-[var(--ink-2)]">
          {range.min === null && range.max === null ? "any" : `${format(lo)} – ${format(hi)}`}
        </span>
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
          <Slider.Track className="relative h-1.5 w-full grow rounded-full bg-muted">
            <Slider.Indicator className="absolute h-full rounded-full bg-primary" />
            {(["Min", "Max"] as const).map((side) => (
              <Slider.Thumb
                key={side}
                aria-label={`${legend} ${side.toLowerCase()}`}
                getAriaValueText={(_formatted, value) => format(value)}
                className="block size-4 shrink-0 rounded-full border border-primary bg-background shadow-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
              />
            ))}
          </Slider.Track>
        </Slider.Control>
      </Slider.Root>
    </div>
  );
}
