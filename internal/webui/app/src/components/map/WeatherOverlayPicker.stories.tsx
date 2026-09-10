/**
 * The picker on its own, with the loading ring switchable from the controls:
 * the ring only ever shows while a grid query is in flight, which no amount of
 * clicking in a story without a map behind it will produce.
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
function Bench({ loading, expanded: initiallyExpanded }: { loading: boolean; expanded: boolean }) {
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
  args: { loading: false, expanded: false },
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

/** The ring runs its lap around a border that stays where it is. */
export const Loading: Story = {
  args: { loading: true },
  parameters: { chromatic: { disableSnapshot: true } },
};
