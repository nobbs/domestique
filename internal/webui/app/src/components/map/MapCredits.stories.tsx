import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent } from "storybook/test";
import { vi } from "vitest";
import { StoryProviders } from "../../storybook/fixtures";
import { MapCredits, type MapCreditsProps } from "./MapCredits";

const SURFACE_CREDIT = "Surface data © OpenStreetMap contributors (ODbL)";

/** What a caller supplies, the fold choice aside: that one is the caller's. */
type CreditProps = Omit<MapCreditsProps, "choice" | "onChoiceChange">;

/**
 * Holds the fold choice, as the map does.
 *
 * The component reports a press rather than remembering it, so a play function
 * that presses the button needs somewhere to see the press land.
 */
function Credits(props: Partial<CreditProps>) {
  const [choice, setChoice] = useState<boolean | null>(null);

  return (
    <MapCredits
      styleUrl={undefined}
      extra={SURFACE_CREDIT}
      {...props}
      choice={choice}
      onChoiceChange={setChoice}
    />
  );
}

const meta = {
  title: "Components/Map/Map Credits",
  component: MapCredits,
  tags: ["autodocs"],
  args: { styleUrl: undefined, choice: null, onChoiceChange: () => {} },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="w-96 bg-[var(--base)] p-6">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof MapCredits>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Credits /> };

export const StartsFolded: Story = {
  render: () => <Credits />,
  play: async ({ canvas }) => {
    await expect(canvas.queryByText(SURFACE_CREDIT)).not.toBeInTheDocument();
    await expect(canvas.getByRole("button", { name: "Show the map credit" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  },
};

export const OpensByKeyboard: Story = {
  render: () => <Credits />,
  play: async ({ canvas }) => {
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Show the map credit" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");

    const credit = canvas.getByText(SURFACE_CREDIT);
    await expect(credit).toBeInTheDocument();
    // The button points at what it just revealed, and only now that it exists.
    await expect(canvas.getByRole("button", { name: "Hide the map credit" })).toHaveAttribute(
      "aria-controls",
      credit.id,
    );
  },
};

export const OpensByClick: Story = {
  render: () => <Credits />,
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Show the map credit" }));

    await expect(canvas.getByText(SURFACE_CREDIT)).toBeInTheDocument();
  },
};

/**
 * The map draws the credit where it stands until the map reports having a
 * control cluster, and into that cluster afterwards. React unmounts and
 * remounts it across that switch, so a press made while the map was still
 * loading would be undone by the map finishing — unless the choice is held
 * outside, which is what this asserts.
 */
export const SurvivesMovingIntoTheCluster: Story = {
  render: () => {
    function Moving() {
      const [choice, setChoice] = useState<boolean | null>(null);
      const [moved, setMoved] = useState(false);
      const credit = (
        <MapCredits
          styleUrl={undefined}
          extra={SURFACE_CREDIT}
          choice={choice}
          onChoiceChange={setChoice}
        />
      );

      return (
        <>
          <button type="button" onClick={() => setMoved(true)}>
            The map found its cluster
          </button>
          {moved ? <section>{credit}</section> : credit}
        </>
      );
    }

    return <Moving />;
  },
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Show the map credit" }));
    await expect(canvas.getByText(SURFACE_CREDIT)).toBeInTheDocument();

    await userEvent.click(canvas.getByRole("button", { name: "The map found its cluster" }));

    await expect(canvas.getByText(SURFACE_CREDIT)).toBeInTheDocument();
  },
};

/** The credit read out of the style document, as text, beside the surface one. */
export const ReadFromTheStyleDocument: Story = {
  render: () => {
    // Stubbed here rather than in `play`: the query fires as soon as the
    // component mounts, which for a story is before `play` runs at all — a
    // `render` function's body still runs before that mount, so this is the
    // last point a play function's own stub could still land in time.
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              sources: {
                basemap: { attribution: '<a href="https://example.test">&copy; Tile People</a>' },
              },
            }),
          ),
      ),
    );

    return <Credits styleUrl="https://tiles.example.test/style.json" />;
  },
  play: async ({ canvas }) => {
    try {
      await userEvent.click(canvas.getByRole("button", { name: "Show the map credit" }));
      await expect(
        await canvas.findByText(`© Tile People · ${SURFACE_CREDIT}`),
      ).toBeInTheDocument();
      // The provider's own markup is read for its words and never rendered.
      await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
    } finally {
      vi.unstubAllGlobals();
    }
  },
};

/** Nothing to give, nothing drawn. */
export const NoCreditToGive: Story = {
  render: () => <Credits extra={undefined} />,
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
  },
};
