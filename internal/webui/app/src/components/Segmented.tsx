/**
 * The application's one segmented control: a muted track, the unselected
 * segments merged into a darker pill either side of the chosen one, and a
 * white thumb that slides to whichever segment is chosen.
 *
 * `SegmentedTrack` is the track, the pills and the thumb behind whatever
 * segments it is given — plain buttons, or a tab list whose panels depend on
 * it — each marked `data-segment` so the track can find and measure them.
 * `Segmented` is the common case: one button per item, `aria-pressed` on the
 * chosen one.
 */

import { type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";

/** The clear ground between the thumb and the pill beside it, in pixels. */
const GAP = 3;
/** The track's own padding, in pixels: where the pills and the thumb begin. */
const INSET = 3;
const RADIUS = 9;

export type Orientation = "horizontal" | "vertical";

/**
 * The segment's own look: quiet until chosen, when its ink darkens and its
 * weight steps up. Both `aria-pressed` and `aria-selected` count, so a tab
 * list wears it as well as a button group.
 */
export function segmentClass(size: "md" | "sm" = "md"): string {
  return cn(
    "relative z-10 flex items-center gap-1.5 rounded-[9px] text-[var(--ink-2)] transition-colors duration-200 hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)] disabled:pointer-events-none disabled:opacity-50 aria-pressed:font-semibold aria-pressed:text-[var(--ink)] aria-[selected=true]:font-semibold aria-[selected=true]:text-[var(--ink)]",
    size === "sm" ? "h-6 px-2.5 text-xs" : "h-7 px-3 text-sm",
  );
}

/**
 * A label over an invisible bold copy of itself, so a segment is as wide
 * regular as bold and choosing it moves none of its neighbours.
 */
export function SegmentLabel({ children }: { children: ReactNode }) {
  return (
    <span className="grid">
      <span className="col-start-1 row-start-1 whitespace-nowrap">{children}</span>
      <span
        aria-hidden="true"
        className="invisible col-start-1 row-start-1 whitespace-nowrap font-semibold"
      >
        {children}
      </span>
    </span>
  );
}

interface Thumb {
  start: number;
  length: number;
  end: number;
}

export interface SegmentedTrackProps {
  /** The `data-segment` of the chosen segment. */
  active: string;
  orientation?: Orientation;
  className?: string | undefined;
  children: ReactNode;
}

const PILL = "color-mix(in oklab, var(--ink-2) 28%, var(--panel))";
const EASE = "duration-200 ease-[cubic-bezier(0.2,0,0,1)] motion-reduce:transition-none";

export function SegmentedTrack({
  active,
  orientation = "horizontal",
  className,
  children,
}: SegmentedTrackProps) {
  const frame = useRef<HTMLDivElement>(null);
  const [thumb, setThumb] = useState<Thumb | null>(null);
  const vertical = orientation === "vertical";

  // biome-ignore lint/correctness/useExhaustiveDependencies: the children are what is measured, so a change in them is a reason to measure again
  useLayoutEffect(() => {
    const measure = () => {
      const segments = frame.current?.querySelectorAll<HTMLElement>("[data-segment]");
      const chosen = frame.current?.querySelector<HTMLElement>(
        `[data-segment="${CSS.escape(active)}"]`,
      );
      const last = segments?.[segments.length - 1];
      if (!chosen || !last) {
        setThumb(null);
        return;
      }
      setThumb(
        vertical
          ? {
              start: chosen.offsetTop,
              length: chosen.offsetHeight,
              end: last.offsetTop + last.offsetHeight,
            }
          : {
              start: chosen.offsetLeft,
              length: chosen.offsetWidth,
              end: last.offsetLeft + last.offsetWidth,
            },
      );
    };
    measure();
    if (typeof ResizeObserver === "undefined" || !frame.current) {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(frame.current);
    return () => observer.disconnect();
  }, [active, vertical, children]);

  // Both pills are the whole band, revealed by a clip that moves with the
  // thumb: a clip keeps its rounded ends at any length, where a shrinking box
  // would squash them.
  const span = thumb ? thumb.end - INSET : 0;
  const before = thumb ? Math.max(span - (thumb.start - GAP - INSET), 0) : span;
  const after = thumb ? Math.max(thumb.start + thumb.length + GAP - INSET, 0) : span;
  const band = vertical
    ? { left: INSET, right: INSET, top: INSET, height: span }
    : { top: INSET, bottom: INSET, left: INSET, width: span };

  return (
    <div
      ref={frame}
      className={cn(
        "relative inline-flex w-fit gap-[3px] rounded-[12px] bg-[var(--muted)] p-[3px]",
        vertical && "flex-col",
        className,
      )}
    >
      {thumb && thumb.length > 0 ? (
        <>
          <div
            aria-hidden="true"
            className={cn("absolute rounded-[9px] transition-[clip-path]", EASE)}
            style={{
              ...band,
              background: PILL,
              clipPath: vertical
                ? `inset(0 0 ${before}px 0 round ${RADIUS}px)`
                : `inset(0 ${before}px 0 0 round ${RADIUS}px)`,
            }}
          />
          <div
            aria-hidden="true"
            className={cn("absolute rounded-[9px] transition-[clip-path]", EASE)}
            style={{
              ...band,
              background: PILL,
              clipPath: vertical
                ? `inset(${after}px 0 0 0 round ${RADIUS}px)`
                : `inset(0 0 0 ${after}px round ${RADIUS}px)`,
            }}
          />
          <div
            aria-hidden="true"
            className={cn(
              "absolute rounded-[9px] bg-[var(--panel)] shadow-[0_0_0_1px_var(--rule),var(--shadow)] transition-[transform,width,height]",
              EASE,
            )}
            style={
              vertical
                ? {
                    left: INSET,
                    right: INSET,
                    top: 0,
                    height: thumb.length,
                    transform: `translateY(${thumb.start}px)`,
                  }
                : {
                    top: INSET,
                    bottom: INSET,
                    left: 0,
                    width: thumb.length,
                    transform: `translateX(${thumb.start}px)`,
                  }
            }
          />
        </>
      ) : null}
      {children}
    </div>
  );
}

export interface SegmentedItem<K extends string> {
  key: K;
  label: ReactNode;
  /** A mark before the label. */
  icon?: ReactNode;
  disabled?: boolean;
}

export interface SegmentedProps<K extends string> {
  /** Names the group for assistive technology. */
  label: string;
  items: readonly SegmentedItem<K>[];
  value: K;
  onChange: (next: K) => void;
  size?: "md" | "sm";
  className?: string | undefined;
}

/** One button per item; pressing the pressed one is a no-op, since one is always chosen. */
export function Segmented<K extends string>({
  label,
  items,
  value,
  onChange,
  size = "md",
  className,
}: SegmentedProps<K>) {
  return (
    <SegmentedTrack active={value} className={className}>
      <div role="group" aria-label={label} className="contents">
        {items.map((item) => (
          <button
            key={item.key}
            type="button"
            data-segment={item.key}
            aria-pressed={item.key === value}
            disabled={item.disabled}
            onClick={() => onChange(item.key)}
            className={segmentClass(size)}
          >
            {item.icon}
            <SegmentLabel>{item.label}</SegmentLabel>
          </button>
        ))}
      </div>
    </SegmentedTrack>
  );
}
