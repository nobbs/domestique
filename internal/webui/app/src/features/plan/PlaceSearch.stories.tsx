import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { PlaceMatch } from "../../api/types";
import { StoryProviders, StubbedFetch } from "../../storybook/fixtures";
import { PlaceSearch } from "./PlaceSearch";

const PLACES: PlaceMatch[] = [
  {
    name: "Turmberg",
    context: "Durlach, Karlsruhe",
    kind: "peak",
    latitude: 49.0006,
    longitude: 8.4868,
  },
  {
    name: "Karlsruhe Hauptbahnhof",
    context: "Südweststadt, Karlsruhe",
    kind: "station",
    latitude: 48.9937,
    longitude: 8.4017,
  },
  {
    name: "Kaiserstraße 12",
    context: "Innenstadt-Ost, Karlsruhe",
    kind: "address",
    latitude: 49.0094,
    longitude: 8.4121,
  },
  {
    name: "Ettlingen",
    context: "Landkreis Karlsruhe, Baden-Württemberg",
    kind: "settlement",
    latitude: 48.9414,
    longitude: 8.4077,
  },
  {
    name: "Schloss Karlsruhe",
    context: "Karlsruhe",
    kind: "place",
    latitude: 49.0134,
    longitude: 8.4044,
  },
];

/** Answers a search with the invented places whose name or surroundings hold the query. */
const respond: typeof fetch = async (input) => {
  const query = new URL(String(input), window.location.href).searchParams.get("query") ?? "";
  const needle = query.toLowerCase();
  const places = PLACES.filter((place) =>
    `${place.name} ${place.context ?? ""}`.toLowerCase().includes(needle),
  );
  return new Response(JSON.stringify({ places }), {
    headers: { "Content-Type": "application/json" },
  });
};

const meta = {
  title: "Features/Planner/Place search",
  component: PlaceSearch,
  tags: ["autodocs"],
  args: { onAdd: () => {} },
  decorators: [
    (Story) => (
      <StoryProviders>
        <StubbedFetch respond={respond}>
          <Story />
        </StubbedFetch>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof PlaceSearch>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Closed: Story = {};

/** A search answers below the field, the nearest match highlighted for Enter. */
export const Searching: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: /Search places/ }));
    const panel = within(document.body);
    await userEvent.type(panel.getByRole("searchbox", { name: "Search for a place" }), "karlsruhe");

    await expect(await panel.findByRole("option", { name: /Turmberg/ })).toBeVisible();
  },
};
