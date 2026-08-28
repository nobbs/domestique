import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { SourceRouteLink } from "./SourceRouteLink";

const meta = {
  title: "Components/Source Route Link",
  component: SourceRouteLink,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SourceRouteLink>;

export default meta;
type Story = StoryObj<typeof meta>;

export const VeloPlanner: Story = {
  args: { provider: "veloplanner", baseUrl: "https://veloplanner.com", sourceRouteId: 12 },
};

/**
 * What every configured link is: named so its destination is clear without
 * seeing it, spoken rather than only shown, reachable by keyboard, and
 * leaving in a new tab without handing the provider a referrer. A stage is
 * not addressable at the provider, so the name promises the route only.
 */
export const Asserted: Story = {
  args: {
    provider: "veloplanner",
    baseUrl: "https://source.example.test",
    sourceRouteId: 4212,
  },
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", {
      name: "Open source route 4212 on source.example.test in a new tab",
    });

    await expect(link).toHaveAttribute("href", "https://source.example.test/user-routes/4212");
    // A stage is not addressable at the provider, so the affordance must not
    // promise precision the destination cannot keep.
    await expect(link.getAttribute("aria-label")).not.toContain("stage");
    // What a reader cannot work out from the row it sits in is where it goes,
    // so that is what the visible label spends its width on.
    await expect(link.textContent).toContain("source.example.test");
    await expect(link).toHaveAttribute("target", "_blank");
    await expect(link.getAttribute("rel")).toContain("noreferrer");
    // Keyboard reachable by being a real link, rather than something clickable
    // that a Tab key walks straight past.
    await expect(link).not.toHaveAttribute("tabindex", "-1");
    await expect(link.tagName).toBe("A");
  },
};

/**
 * A `www.` and a non-default port: the visible label and the spoken name both
 * still say the plain host a reader would recognise, even though the address
 * they follow keeps the rest.
 */
export const WwwAndPort: Story = {
  args: {
    provider: "veloplanner",
    baseUrl: "https://www.source.example.test:8443",
    sourceRouteId: 4212,
  },
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link");

    await expect(link.textContent).toContain("source.example.test");
    await expect(link.getAttribute("aria-label")).toContain("source.example.test");
  },
};

/** No configured base URL: nothing to offer, so nothing is drawn. */
export const NoConfiguredProvider: Story = {
  args: { provider: "veloplanner", baseUrl: undefined, sourceRouteId: 4212 },
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
  },
};

/** A base URL configured but unusable: still nothing, rather than a dead link. */
export const UnusableBaseUrl: Story = {
  args: { provider: "veloplanner", baseUrl: "not-a-url", sourceRouteId: 4212 },
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("link")).toBeNull();
  },
};

/** A configured base URL is not enough on its own: an unknown provider gets no guessed link either. */
export const UnknownProvider: Story = {
  args: { provider: "komoot", baseUrl: "https://source.example.test", sourceRouteId: 4212 },
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("link")).toBeNull();
  },
};
