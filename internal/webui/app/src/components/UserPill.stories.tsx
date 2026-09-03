import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, screen, userEvent, within } from "storybook/test";
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

function config(): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    identity: { display: "alexej.disterhoft@example.test", admin: false },
  };
}

/** A signed-in reader, named so the session can be seen to be theirs. */
export const SignedIn: Story = {
  decorators: withConfig(config()),
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
    // Presence, not visibility: the popover is still animating open here. The
    // sign-out itself is exercised end to end by e2e/contract/sign-in.spec.ts.
    await expect(screen.getByRole("button", { name: "Sign out" })).toBeEnabled();
  },
};

/** An admin session, which alone is offered the rider-view preview switch. */
export const Admin: Story = {
  decorators: withConfig({ ...config(), identity: { display: "admin@example.test", admin: true } }),
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: /Signed in as/ }));

    const menu = await screen.findByRole("dialog", { name: "Session" });
    await expect(within(menu).getByRole("switch", { name: "View as rider" })).toBeInTheDocument();
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
