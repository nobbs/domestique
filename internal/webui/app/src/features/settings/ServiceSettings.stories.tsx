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

/**
 * A section is saved on its own, to the endpoint that owns it, carrying the
 * whole of that section and nothing from any other.
 */
export const SavesOneSectionOnItsOwn: Story = {
  decorators: [
    (Story) => (
      <StubbedFetch respond={respond}>
        <Story />
      </StubbedFetch>
    ),
  ],
  play: async ({ canvas }) => {
    written.length = 0;
    const hours = canvas.getByLabelText("Call the library stale after (hours)");
    await userEvent.clear(hours);
    await userEvent.type(hours, "30");

    await userEvent.click(canvas.getByRole("button", { name: "Save Sync" }));

    await waitFor(() => expect(written).toHaveLength(1));
    await expect(written[0]?.url).toContain("/v1/settings/sync");
    // Hours on the page, seconds on the wire.
    await expect(written[0]?.body).toEqual({
      allowEmptySourceDeletion: false,
      staleAfterSeconds: 30 * 3600,
      initialDelaySeconds: 60,
    });
    await expect(await canvas.findByText(/^Saved\./)).toBeVisible();
  },
};

/**
 * Only the credential that was typed into is sent, and it goes to the section
 * that owns it rather than to a page-wide list of credentials.
 */
export const SendsOnlyTheCredentialThatWasTyped: Story = {
  decorators: [
    (Story) => (
      <StubbedFetch respond={respond}>
        <Story />
      </StubbedFetch>
    ),
  ],
  play: async ({ canvas }) => {
    written.length = 0;
    await userEvent.type(canvas.getByLabelText("Komoot password"), "opensesame");

    await userEvent.click(canvas.getByRole("button", { name: "Save Komoot" }));

    await waitFor(() => expect(written).toHaveLength(1));
    await expect(written[0]?.url).toContain("/v1/settings/sources/komoot");
    await expect(written[0]?.body.password).toBe("opensesame");
    await expect(written[0]?.body.email).toBeUndefined();
  },
};

/**
 * The alert matrix draws what this build can announce, with an alert nobody has
 * ruled on shown as on. Only the switches that were moved are sent, so leaving
 * one alone keeps whatever it had.
 */
export const SendsOnlyTheAlertsThatWereSwitched: Story = {
  decorators: [
    (Story) => (
      <StubbedFetch respond={respond}>
        <Story />
      </StubbedFetch>
    ),
  ],
  play: async ({ canvas }) => {
    written.length = 0;
    await expect(canvas.getByRole("switch", { name: "sync source" })).toBeChecked();
    await expect(canvas.getByRole("switch", { name: "sync destination" })).not.toBeChecked();

    // Off and on again is where it started, so it is not a decision to send.
    await userEvent.click(canvas.getByRole("switch", { name: "sync source" }));
    await userEvent.click(canvas.getByRole("switch", { name: "sync source" }));
    await userEvent.click(canvas.getByRole("switch", { name: "surface:index build" }));

    await userEvent.click(canvas.getByRole("button", { name: "Save Alerts" }));

    await waitFor(() => expect(written).toHaveLength(1));
    await expect(written[0]?.url).toContain("/v1/settings/alerts");
    await expect(written[0]?.body).toEqual({
      alerts: [{ task: "surface:index", alert: "build", enabled: false }],
    });
  },
};

interface Written {
  url: string;
  // biome-ignore lint/suspicious/noExplicitAny: the body under assertion is one section of a settings document
  body: any;
}

const written: Written[] = [];

/** Records the write and answers both it and the read that follows it. */
const respond: typeof fetch = async (input, init) => {
  if (init?.method === "PUT") {
    written.push({ url: String(input), body: JSON.parse(String(init.body)) });
  }

  return new Response(JSON.stringify(settings), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
};
