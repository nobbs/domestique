/**
 * A segmented control whose unselected segments merge into one band and
 * whose selected thumb slides between positions.
 *
 * Storybook only: nothing here is imported by the application.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLayoutEffect, useRef, useState } from "react";
import { StoryProviders } from "../storybook/fixtures";

interface Item<K extends string> {
  key: K;
  label: string;
  count?: number;
}

/** The clear ground between the thumb and the pill beside it, in pixels. */
const GAP = 3;

interface SegmentedProps<K extends string> {
  items: readonly Item<K>[];
  value: K;
  onChange: (next: K) => void;
  /** Milliseconds the thumb takes to reach the chosen segment; 0 for none. */
  duration?: number | undefined;
}

/**
 * The track is the muted tone. The unselected segments do not paint
 * themselves: the run to the thumb's left shares one darker pill and the run
 * to its right another, and the white thumb slides between them, all of it
 * measured rather than computed so a label of any length is fine.
 */
function Segmented<K extends string>({
  items,
  value,
  onChange,
  duration = 220,
}: SegmentedProps<K>) {
  const buttons = useRef(new Map<K, HTMLButtonElement>());
  const [thumb, setThumb] = useState<{ left: number; width: number; end: number } | null>(null);

  useLayoutEffect(() => {
    const chosen = buttons.current.get(value);
    const last = buttons.current.get(items[items.length - 1]?.key as K);
    if (!chosen || !last) {
      return;
    }
    const measure = () =>
      setThumb({
        left: chosen.offsetLeft,
        width: chosen.offsetWidth,
        end: last.offsetLeft + last.offsetWidth,
      });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(chosen);
    observer.observe(last);
    return () => observer.disconnect();
  }, [value, items]);

  const ease = `${duration}ms cubic-bezier(0.2, 0, 0, 1)`;
  const band = "color-mix(in oklab, var(--ink-2) 28%, var(--panel))";
  // The unselected segments to the thumb's left share one pill, and those to
  // its right another; each shrinks to nothing when the thumb is at that end.
  const first = 3;
  const leftWidth = thumb ? Math.max(thumb.left - GAP - first, 0) : 0;
  const rightStart = thumb ? thumb.left + thumb.width + GAP : 0;
  const rightWidth = thumb ? Math.max(thumb.end - rightStart, 0) : 0;

  return (
    <div
      role="tablist"
      className="relative inline-flex w-fit rounded-[12px] bg-[var(--muted)] p-[3px]"
    >
      {thumb ? (
        <>
          <div
            aria-hidden="true"
            className="absolute top-[3px] bottom-[3px] rounded-[9px]"
            style={{
              left: first,
              width: leftWidth,
              background: band,
              transition: `width ${ease}`,
            }}
          />
          <div
            aria-hidden="true"
            className="absolute top-[3px] bottom-[3px] rounded-[9px]"
            style={{
              left: 0,
              width: rightWidth,
              transform: `translateX(${rightStart}px)`,
              background: band,
              transition: `transform ${ease}, width ${ease}`,
            }}
          />
          {/* The thumb, under the labels and over the pills. */}
          <div
            aria-hidden="true"
            className="absolute top-[3px] bottom-[3px] rounded-[9px] bg-[var(--panel)] shadow-[0_0_0_1px_var(--rule),var(--shadow)]"
            style={{
              left: 0,
              width: thumb.width,
              transform: `translateX(${thumb.left}px)`,
              transition: `transform ${ease}, width ${ease}`,
            }}
          />
        </>
      ) : null}
      {items.map((item) => {
        const on = item.key === value;
        return (
          <button
            key={item.key}
            ref={(node) => {
              if (node) {
                buttons.current.set(item.key, node);
              } else {
                buttons.current.delete(item.key);
              }
            }}
            type="button"
            role="tab"
            aria-selected={on}
            onClick={() => onChange(item.key)}
            className="relative z-10 flex h-7 items-center gap-1.5 rounded-[9px] px-3 text-sm transition-colors duration-200 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
            style={{ color: on ? "var(--ink)" : "var(--ink-2)", fontWeight: on ? 600 : 400 }}
          >
            {item.label}
            {item.count !== undefined ? (
              <span
                className="rounded-full px-1.5 py-px text-[11px] tabular-nums transition-colors duration-200"
                style={
                  on
                    ? { background: "var(--ink)", color: "var(--panel)" }
                    : {
                        background: "color-mix(in oklab, var(--ink-2) 18%, transparent)",
                        color: "var(--ink)",
                      }
                }
              >
                {item.count}
              </span>
            ) : null}
          </button>
        );
      })}
    </div>
  );
}

const ITEMS = [
  { key: "calendar", label: "Calendar" },
  { key: "waitlist", label: "Waitlist", count: 14 },
  { key: "requests", label: "Requests", count: 10 },
  { key: "cancellations", label: "Cancellations" },
  { key: "payers", label: "Payers" },
] as const;

type Key = (typeof ITEMS)[number]["key"];

const meta = {
  title: "Spikes/Segmented Control",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Demo({ duration }: { duration?: number | undefined }) {
  const [value, setValue] = useState<Key>("waitlist");
  return <Segmented items={ITEMS} value={value} onChange={setValue} duration={duration} />;
}

export const FiveTabs: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-8 bg-[var(--base)] p-6">
        <div className="grid gap-2">
          <h2 className="font-semibold text-sm">On the page ground</h2>
          <Demo />
        </div>
        <div className="grid gap-2 rounded-2xl bg-[var(--panel)] p-4 shadow-[var(--shadow)]">
          <h2 className="font-semibold text-sm">Inside a card</h2>
          <Demo />
        </div>
        <div className="grid gap-2">
          <h2 className="font-semibold text-sm">Without the animation</h2>
          <Demo duration={0} />
        </div>
      </div>
    </StoryProviders>
  ),
};
