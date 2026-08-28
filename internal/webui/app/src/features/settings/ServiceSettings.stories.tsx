import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { StoryProviders, StubbedFetch, settings } from "../../storybook/fixtures";
import { ServiceSettings } from "./ServiceSettings";

const meta = {
  title: "Features/Service settings",
  component: ServiceSettings,
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
} satisfies Meta<typeof ServiceSettings>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Every field carries what the service answered, in the unit it is set in. */
export const WhatTheServiceIsSetTo: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByLabelText("Call the library stale after (hours)")).toHaveValue(26);
    await expect(canvas.getByLabelText("Geofabrik regions, one per line")).toHaveValue(
      "europe/germany",
    );
    await expect(canvas.getByRole("radio", { name: "One summary per period" })).toBeChecked();
  },
};

/**
 * The switch that lets a run delete a whole library asks first, and says that
 * it stays on until it is turned off again.
 */
export const AsksBeforeAnEmptyLibraryMayDelete: Story = {
  play: async ({ canvas }) => {
    const control = canvas.getByRole("switch", {
      name: "Let an empty library delete a target's routes",
    });
    await expect(control).not.toBeChecked();

    await userEvent.click(control);

    // The dialog is portalled out of the story's canvas.
    const dialog = within(document.body).getByRole("alertdialog");
    await expect(dialog).toHaveTextContent("It stays on until you turn it off again.");

    await userEvent.click(within(dialog).getByRole("button", { name: "Allow it" }));

    await expect(control).toBeChecked();
  },
};

/**
 * A credential is offered for replacement rather than shown, because the page
 * is never told what the service holds — only that it holds one.
 */
export const OffersToReplaceACredentialItWasNeverTold: Story = {
  play: async ({ canvas }) => {
    const stored = canvas.getByLabelText("Client secret");
    await expect(stored).toHaveValue("");
    await expect(stored).toHaveAttribute("placeholder", "Stored — type to replace");

    await expect(canvas.getByLabelText("Pushover application token")).toHaveValue("");
  },
};

/** Only the credentials typed into are sent; the rest are left as they are. */
export const SendsOnlyTheCredentialThatWasTyped: Story = {
  decorators: [
    (Story) => (
      <StubbedFetch respond={respond}>
        <Story />
      </StubbedFetch>
    ),
  ],
  play: async ({ canvas }) => {
    sent.length = 0;
    await userEvent.type(canvas.getByLabelText("Client secret"), "replacement");

    await userEvent.click(canvas.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    const body = JSON.parse(String(sent[0]?.body));
    await expect(body.secrets).toEqual({ "wahoo.client_secret": "replacement" });
  },
};

/** Nothing is changed until it is saved, and what is saved is the whole object. */
export const SavesEverySettingAtOnce: Story = {
  decorators: [
    (Story) => (
      <StubbedFetch respond={respond}>
        <Story />
      </StubbedFetch>
    ),
  ],
  play: async ({ canvas }) => {
    sent.length = 0;
    const hours = canvas.getByLabelText("Call the library stale after (hours)");
    await userEvent.clear(hours);
    await userEvent.type(hours, "30");

    await userEvent.click(canvas.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    const body = JSON.parse(String(sent[0]?.body));
    await expect(Object.keys(body).sort()).toEqual([
      "basemaps",
      "notifications",
      "rideModel",
      "sources",
      "surface",
      "sync",
      "wahoo",
    ]);
    // Hours on the page, seconds on the wire.
    await expect(body.sync.staleAfterSeconds).toBe(30 * 3600);
    await expect(await canvas.findByText(/^Saved\./)).toBeVisible();
  },
};

const sent: RequestInit[] = [];

/** Records the write and answers both it and the read that follows it. */
const respond: typeof fetch = async (_input, init) => {
  if (init?.method === "PUT") {
    sent.push(init);
  }

  return new Response(JSON.stringify(settings), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
};
