/**
 * The region picker that replaced the textarea of Geofabrik slugs.
 *
 * What it is for is the two things the textarea could not do: name a region
 * from a catalogue rather than from memory, and say what indexing it costs
 * before a week-long rebuild finds out. A well-formed typo used to be accepted
 * in silence and then fail the whole build with no region named.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { StoryProviders } from "../../../storybook/fixtures";
import { CATALOGUE } from "./catalogue.generated";
import { RegionPicker } from "./RegionPicker";

/** Holds the selection, as the settings card does. */
function Picker({ initial }: { initial: string[] }) {
  const [value, setValue] = useState(initial);

  return (
    <div className="max-w-2xl rounded-xl border border-[var(--rule)] bg-[var(--panel)] p-5">
      <RegionPicker value={value} onChange={setValue} />
    </div>
  );
}

const meta = {
  title: "Features/Region picker",
  component: Picker,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="bg-[var(--base)] p-4">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
  args: { initial: [] },
} satisfies Meta<typeof Picker>;

export default meta;

type Story = StoryObj<typeof meta>;

/** Nothing chosen: Germany is what is offered before anything is typed. */
export const NothingChosen: Story = {};

/** The whole country, and the 4.5 GB that asking for it costs. */
export const TheWholeCountry: Story = { args: { initial: ["europe/germany"] } };

/** Three neighbouring states, which together cost a seventh of the country. */
export const SeveralStates: Story = {
  args: {
    initial: ["europe/germany/hessen", "europe/germany/rheinland-pfalz", "europe/germany/saarland"],
  },
};

/** A region is found by its name, which is not always how its slug is spelled. */
export const FoundByName: Story = {
  play: async ({ canvas }: { canvas: ReturnType<typeof within> }) => {
    await userEvent.type(canvas.getByLabelText("Regions to index"), "württemberg");
    await expect(await canvas.findByText("europe/germany/baden-wuerttemberg")).toBeVisible();
  },
};

/**
 * Choosing a region drops whatever it already contains.
 *
 * Asserted against the chips rather than the whole card: a state that stops
 * being selected reappears immediately in the matches below, so the slug is
 * still on the page — which is the point, not a leak.
 */
export const ChoosingACountryDropsItsStates: Story = {
  args: { initial: ["europe/germany/bayern"] },
  play: async ({ canvas }: { canvas: ReturnType<typeof within> }) => {
    await userEvent.click(await canvas.findByText("europe/germany"));
    const chosen = within(canvas.getByRole("list", { name: "Selected regions" }));
    await expect(chosen.getByText("europe/germany")).toBeVisible();
    await expect(chosen.queryByText("europe/germany/bayern")).not.toBeInTheDocument();
  },
};

/**
 * Geofabrik states no size for a handful of its regions — a few islands and
 * territories. They stay selectable, and the totals say they are a floor rather
 * than counting what they cannot see as nothing.
 *
 * The region is taken from the catalogue rather than named here, so this keeps
 * testing the behaviour if Geofabrik starts publishing a size for whichever one
 * would otherwise have been hardcoded.
 */
const UNPRICED = CATALOGUE.find((entry) => entry.bytes === null)?.slug ?? "";

export const ARegionWithNoPublishedSize: Story = {
  args: { initial: ["europe/germany/saarland", UNPRICED] },
  play: async ({ canvas }: { canvas: ReturnType<typeof within> }) => {
    await expect(await canvas.findByText(/at least 52 MB downloaded/)).toBeVisible();
    await expect(await canvas.findByText(/no published size/)).toBeVisible();
  },
};

/**
 * A slug the catalogue does not know is kept and flagged rather than dropped:
 * the service still accepts any well-formed region, so an existing setting must
 * survive being opened here.
 */
export const AnUnknownRegionIsKeptAndFlagged: Story = {
  args: { initial: ["europe/germany", "europe/germay"] },
  play: async ({ canvas }: { canvas: ReturnType<typeof within> }) => {
    await expect(await canvas.findByText(/not in the catalogue/)).toBeVisible();
  },
};
