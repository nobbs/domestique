import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { SourceRouteLink } from "./SourceRouteLink";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "./ui/dropdown-menu";

const meta = {
  title: "Components/Source Route Link",
  component: SourceRouteLink,
  tags: ["autodocs"],
  decorators: [
    // The link is a menu item, so it is only itself inside a menu: the role it
    // answers to, the keyboard that reaches it and the width it sits in all
    // come from the menu around it.
    (Story) => (
      <div className="bg-[var(--base)] p-6">
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger aria-label="More about this route">…</DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-auto min-w-52">
            <Story />
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    ),
  ],
} satisfies Meta<typeof SourceRouteLink>;

export default meta;
type Story = StoryObj<typeof meta>;

// The menu is portalled to the end of the document, so the story's own canvas
// is the one place it is not.
const menu = () => within(document.body);

/*
 * The open menu, once it exists.
 *
 * The popup is portalled and positioned after the story renders, so a
 * synchronous query races it: this passed on a fast machine and failed on
 * Chromatic's, which is the worst way for a test to be wrong. Waiting for the
 * menu itself also gives the absence stories something to assert against —
 * "no item" is only meaningful once there is a menu to not contain one.
 */
async function openMenu() {
  return await menu().findByRole("menu");
}

export const VeloPlanner: Story = {
  args: { provider: "veloplanner", baseUrl: "https://veloplanner.com", sourceRouteId: 12 },
};

/**
 * What every configured link is: named so its destination is clear without
 * seeing it, spoken rather than only shown, and leaving in a new tab without
 * handing the provider a referrer. A stage is not addressable at the provider,
 * so the name promises the route only.
 */
export const Asserted: Story = {
  args: {
    provider: "veloplanner",
    baseUrl: "https://source.example.test",
    sourceRouteId: 4212,
  },
  play: async () => {
    const link = await menu().findByRole("menuitem", {
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
    // Still a real anchor under the menu item's role, which is what keeps
    // middle-click, copy-link and the browser's own idea of an outbound
    // address working. The menu's own keys reach it; Tab no longer does, and
    // that is the menu's contract rather than this link's.
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
  play: async () => {
    const link = await menu().findByRole("menuitem");

    await expect(link.textContent).toContain("source.example.test");
    await expect(link.getAttribute("aria-label")).toContain("source.example.test");
  },
};

/** No configured base URL: nothing to offer, so nothing is drawn. */
export const NoConfiguredProvider: Story = {
  args: { provider: "veloplanner", baseUrl: undefined, sourceRouteId: 4212 },
  play: async () => {
    await expect(within(await openMenu()).queryByRole("menuitem")).not.toBeInTheDocument();
  },
};

/** A base URL configured but unusable: still nothing, rather than a dead link. */
export const UnusableBaseUrl: Story = {
  args: { provider: "veloplanner", baseUrl: "not-a-url", sourceRouteId: 4212 },
  play: async () => {
    await expect(within(await openMenu()).queryByRole("menuitem")).toBeNull();
  },
};

/** A configured base URL is not enough on its own: an unknown provider gets no guessed link either. */
export const UnknownProvider: Story = {
  args: { provider: "komoot", baseUrl: "https://source.example.test", sourceRouteId: 4212 },
  play: async () => {
    await expect(within(await openMenu()).queryByRole("menuitem")).toBeNull();
  },
};
