/**
 * The chooser, played through every state `BasemapPicker.test.tsx` used to
 * assert with Testing Library and jsdom — folded, unfolded, a pick reported,
 * Escape closing it. It moved here because there was nothing left for a
 * second render of the same component to earn: the story already stood the
 * component up the way the application does, and a `play` function can ask
 * the same questions of that one render that a separate jsdom test asked of
 * its own.
 *
 * Not every component test made that same move. One that renders react-map-gl
 * itself (`MapControls`, `MapOverlay`, `MapWidget`) mocks it to isolate a
 * narrow question — an imperative call made, a portal target picked — from a
 * WebGL canvas neither jsdom nor this browser can meaningfully render; that
 * isolation is the point, and it stays in Testing Library. So does anything
 * whose real assertions are geometry or gesture math with dozens of cases
 * (`ElevationProfile`, `RouteKey`, `RouteOverlay`) — a real browser proves
 * nothing more about arithmetic than jsdom already does, at several times the
 * cost per case.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, screen, userEvent, waitFor } from "storybook/test";
import type { Basemap } from "../../api/types";
import { BasemapPicker } from "./BasemapPicker";

const streets: Basemap = { name: "Streets", styleUrl: "", darkCartography: false };
const dark: Basemap = { name: "Dark", styleUrl: "", darkCartography: true };
const basemaps = [streets, dark];

/**
 * Holds the fold, as the map does.
 *
 * The component reports a press rather than remembering it, because the portal
 * that moves it into MapLibre's cluster remounts it — so a play function that
 * presses the button needs somewhere to see the press land.
 *
 * The popover it opens renders through a portal into `document.body`, outside
 * this story's own canvas root — which is why every play function below reads
 * it back with `screen` rather than `canvas`, and with `find` rather than
 * `get`. A portal lands a frame or more after whatever put it there, so a
 * synchronous query reads the document before the popover is in it: on a fast
 * machine that is a race these win, and on a slower one it is a failure.
 */
function Picker({
  basemaps: options = basemaps,
  selectedName = streets.name,
  onSelect = fn(),
  initiallyExpanded = true,
}: {
  basemaps?: Basemap[];
  selectedName?: string;
  onSelect?: (name: string) => void;
  initiallyExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(initiallyExpanded);

  return (
    <BasemapPicker
      basemaps={options}
      selectedName={selectedName}
      onSelect={onSelect}
      expanded={expanded}
      onExpandedChange={setExpanded}
    />
  );
}

const meta = {
  title: "Components/Map/Basemap Picker",
  component: BasemapPicker,
  tags: ["autodocs"],
  args: {
    basemaps,
    selectedName: streets.name,
    onSelect: fn(),
    expanded: true,
    onExpandedChange: () => {},
  },
  decorators: [
    (Story) => (
      <div className="w-64 bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof BasemapPicker>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Picker /> };

/** Where the operator configured only one basemap, there is nothing to choose. */
export const SingleBasemap: Story = {
  render: () => <Picker basemaps={[streets]} />,
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
  },
};

export const Folded: Story = {
  render: () => <Picker initiallyExpanded={false} />,
  play: async ({ canvas }) => {
    await expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
    await expect(canvas.getByRole("button", { name: "Choose the basemap" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  },
};

export const Unfolded: Story = {
  render: () => <Picker initiallyExpanded={false} />,
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Choose the basemap" }));

    const group = await screen.findByRole("radiogroup", { name: "Basemap" });
    await expect(group).toBeInTheDocument();
    await expect(screen.getByRole("radio", { name: "Streets" })).toBeInTheDocument();
    await expect(screen.getByRole("radio", { name: "Dark" })).toBeInTheDocument();
    await expect(screen.getAllByRole("radio")).toHaveLength(2);

    // The name is the whole of what the button says, so it has to change with
    // the fold: the mark inside it does not, and `aria-expanded` alone would
    // leave a reader who cannot see the list guessing what pressing it does
    // next. What it opens is the popover, and the names are inside that — the
    // group is no longer a sibling the button can name directly.
    const toggle = canvas.getByRole("button", { name: "Hide the basemap choices" });
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    const controls = toggle.getAttribute("aria-controls") ?? "";
    await expect(document.getElementById(controls)).toContainElement(group);
  },
};

/**
 * A reader who has seen enough gets out without aiming at the mark again —
 * the reason this stopped being a panel that unfolds in place.
 */
export const ClosesOnEscape: Story = {
  render: () => <Picker initiallyExpanded={false} />,
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Choose the basemap" }));
    await screen.findByRole("radiogroup");
    await userEvent.keyboard("{Escape}");

    // The keydown resolving is not the state update it triggers resolving:
    // the popover closes on the next render, one tick after the key press
    // userEvent has already returned from.
    await waitFor(() => expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument());
  },
};

export const MarksTheLoadedBasemap: Story = {
  render: () => <Picker selectedName={dark.name} />,
  play: async () => {
    await expect(await screen.findByRole("radio", { name: "Dark" })).toBeChecked();
    await expect(screen.getByRole("radio", { name: "Streets" })).not.toBeChecked();
  },
};

export const ReportsAPickByName: Story = {
  render: (args) => <Picker onSelect={args.onSelect} />,
  play: async ({ args }) => {
    await userEvent.click(await screen.findByRole("radio", { name: "Dark" }));

    await expect(args.onSelect).toHaveBeenCalledWith("Dark");
  },
};

/**
 * The mark follows the ground actually loaded rather than the press: a name
 * the operator has since dropped falls back to the first entry, and the
 * checked radio has to say so rather than claiming a basemap that is not on
 * screen.
 */
export const NoMatchingSelection: Story = {
  render: () => <Picker selectedName="Ordnance Survey" />,
  play: async () => {
    await expect(screen.queryAllByRole("radio", { checked: true })).toHaveLength(0);
  },
};
