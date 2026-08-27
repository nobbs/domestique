import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, screen, userEvent } from "storybook/test";
import { webUIConfigQuery } from "../api/queries";
import type { WebUIConfig } from "../api/types";
import { StoryProviders } from "../storybook/fixtures";
import { UserPill } from "./UserPill";

const meta = {
  title: "Components/User Pill",
  component: UserPill,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="flex justify-end bg-[var(--panel)] p-3">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof UserPill>;

/*
 * The menu renders through a portal into `document.body`, outside this story's
 * own canvas root, so every play function below reads it back with `screen`.
 */

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * The configuration each story needs, over the one `StoryProviders` carries.
 * Nested inside that decorator rather than replacing it: `useQuery` reads the
 * closest provider, so the fixture's client is shadowed for this one query and
 * the router it also mounts stays where it is.
 */
function withConfig(value?: WebUIConfig): NonNullable<Meta<typeof UserPill>["decorators"]> {
  return [
    (Story) => {
      // `enabled: false` because every story here seeds what it wants read.
      // Without it the story that seeds nothing — the state before an answer
      // has arrived — is the one story that reaches for the network, and it
      // would be asking a Storybook that serves no API.
      const client = new QueryClient({
        defaultOptions: {
          queries: { enabled: false, retry: false, staleTime: Number.POSITIVE_INFINITY },
        },
      });
      if (value) {
        client.setQueryData(webUIConfigQuery().queryKey, value);
      }

      return (
        <QueryClientProvider client={client}>
          <Story />
        </QueryClientProvider>
      );
    },
  ];
}

function config(signOutUrl?: string): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    identity: { email: "alexej.disterhoft@example.test", ...(signOutUrl ? { signOutUrl } : {}) },
  };
}

/** A deployment behind Cloudflare Access, which is one with a way out. */
export const SignedIn: Story = {
  decorators: withConfig(config("/cdn-cgi/access/logout")),
  play: async ({ canvas }) => {
    const pill = canvas.getByRole("button", {
      name: "Signed in as alexej.disterhoft@example.test",
    });
    // Two letters are an abbreviation; the address is what the control is named
    // by, so a pointer and a screen reader both learn the session without
    // opening anything.
    await expect(pill).toHaveTextContent("AD");

    await userEvent.click(pill);

    const menu = await screen.findByRole("dialog", { name: "Session" });
    await expect(menu).toHaveTextContent("alexej.disterhoft@example.test");
    await expect(screen.getByRole("link", { name: /Sign out/ })).toHaveAttribute(
      "href",
      "/cdn-cgi/access/logout",
    );
  },
};

/**
 * A deployment with nothing in front of it — the demo, and any run where the
 * gate is satisfied by something that serves no logout of its own. The address
 * is still worth saying; the way out is absent rather than dead.
 */
export const NoWayOut: Story = {
  decorators: withConfig(config()),
  play: async ({ canvas }) => {
    await userEvent.click(
      canvas.getByRole("button", { name: "Signed in as alexej.disterhoft@example.test" }),
    );

    const menu = await screen.findByRole("dialog", { name: "Session" });
    await expect(menu).toHaveTextContent("alexej.disterhoft@example.test");
    await expect(screen.queryByRole("link", { name: /Sign out/ })).toBeNull();
  },
};

/**
 * The configuration has not arrived. Nothing is drawn: an empty circle in the
 * corner would be a claim about a session no answer has been given for.
 */
export const NotYetKnown: Story = {
  decorators: withConfig(),
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};
