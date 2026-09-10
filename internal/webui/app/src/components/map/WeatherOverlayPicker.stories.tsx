/**
 * The picker on its own, with the loading ring switchable from the controls —
 * it only ever shows while a grid query is in flight, which no amount of
 * clicking in a story without a map behind it will produce — and its arc and
 * lap tunable there, since the size of an arc that reads as motion is a thing
 * to look at rather than to reason about.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { MEASURES, type MeasureKey } from "../../lib/measures";
import { WeatherOverlayPicker } from "./WeatherOverlayPicker";

/**
 * Holds the picker's state, and a client whose grid query never settles while
 * `loading` is on. Remount it — `key` on the story's render — to change that:
 * the query is seeded once, when the client is made.
 */
function Bench({
  loading,
  expanded: initiallyExpanded,
  arcPercent,
  lapSeconds,
}: {
  loading: boolean;
  expanded: boolean;
  arcPercent: number;
  lapSeconds: number;
}) {
  const [client] = useState(() => {
    const next = new QueryClient();

    if (loading) {
      void next.fetchQuery({
        queryKey: ["wind-grid"],
        queryFn: () => new Promise<number>(() => {}),
      });
    }

    return next;
  });
  const [selected, setSelected] = useState<ReadonlySet<MeasureKey>>(new Set(["wind"]));
  const [hoursAhead, setHoursAhead] = useState(0);
  const [expanded, setExpanded] = useState(initiallyExpanded);

  return (
    <QueryClientProvider client={client}>
      {/* An author rule beats a presentation attribute, so the ring takes these
          without the component growing knobs for values it will end up fixing. */}
      <style>{`.animate-ring-trace {
        stroke-dasharray: ${arcPercent} ${100 - arcPercent};
        animation-duration: ${lapSeconds}s;
      }`}</style>
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={selected}
        onToggle={(key, on) =>
          setSelected((current) => {
            const next = new Set(current);
            on ? next.add(key) : next.delete(key);

            return next;
          })
        }
        hoursAhead={hoursAhead}
        onHoursAheadChange={setHoursAhead}
        expanded={expanded}
        onExpandedChange={setExpanded}
      />
    </QueryClientProvider>
  );
}

const meta = {
  title: "Components/Map/Weather Overlay Picker",
  component: Bench,
  tags: ["autodocs"],
  args: { loading: false, expanded: false, arcPercent: 30, lapSeconds: 1.4 },
  argTypes: {
    arcPercent: { control: { type: "range", min: 5, max: 95, step: 5 } },
    lapSeconds: { control: { type: "range", min: 0.4, max: 4, step: 0.1 } },
  },
  render: (args) => <Bench key={String(args.loading)} {...args} />,
  decorators: [
    (Story) => (
      <div className="w-64 bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof Bench>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Unfolded: Story = { args: { expanded: true } };

/** The ring runs its lap around a border that stays where it is. Arc and lap
 *  are live in the controls; whatever reads best gets baked into the component
 *  and `--animate-ring-trace`. */
export const Loading: Story = {
  args: { loading: true },
  parameters: { chromatic: { disableSnapshot: true } },
};
