/**
 * Five positions on how the Effort card's time in zones should read. Storybook
 * only; see the note on each variant in `effortVariants.tsx` for its bet.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  BandsEffort,
  ColumnsEffort,
  RingEffort,
  RingSeriesEffort,
  RowsEffort,
  SeriesEffort,
} from "./effortVariants";

const meta = {
  title: "Spikes/Effort",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function card(Variant: () => React.JSX.Element): Story {
  return {
    render: () => (
      <div className="min-h-dvh bg-[var(--base)] p-6">
        <div className="max-w-xl">
          <Variant />
        </div>
      </div>
    ),
  };
}

/** A · Rows: one bar per zone, device zones as a hairline beneath. */
export const Rows = card(RowsEffort);
/** B · Columns: a five-column histogram with the device's cut as hollow twins. */
export const Columns = card(ColumnsEffort);
/** C · Bands: easy / moderate / hard shares first, zones as small print. */
export const Bands = card(BandsEffort);
/** D · Ring: total at the centre, a legend table beside. */
export const Ring = card(RingEffort);
/** E · Series: a bpm histogram under the zone edges and a ribbon of when. */
export const Series = card(SeriesEffort);
/** D+E · Ring and table on top, histogram and ribbon below, one zone lit across all three on hover. */
export const RingSeries = card(RingSeriesEffort);

/** D+E at the full width the card has when the ride carries no other figures. */
export const RingSeriesWide: Story = {
  render: () => (
    <div className="min-h-dvh bg-[var(--base)] p-6">
      <div className="max-w-4xl">
        <RingSeriesEffort />
      </div>
    </div>
  ),
};

/** Every variant side by side, for comparison. */
export const All: Story = {
  render: () => (
    <div className="grid min-h-dvh grid-cols-[repeat(auto-fill,minmax(32rem,1fr))] gap-6 bg-[var(--base)] p-6">
      {[RowsEffort, ColumnsEffort, BandsEffort, RingEffort, SeriesEffort, RingSeriesEffort].map(
        (Variant) => (
          <Variant key={Variant.name} />
        ),
      )}
    </div>
  ),
};
