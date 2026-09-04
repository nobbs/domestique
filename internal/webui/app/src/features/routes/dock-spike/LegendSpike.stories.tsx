/**
 * Five looks for the wash key, on the two measures that stretch it most: wind,
 * which has two keys, and rain, whose first band is not washed at all.
 *
 * Storybook only. All five hold the same content and the same tooltips; what
 * differs is how much ink a band spends saying it is a colour.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MEASURES } from "../../../lib/measures";
import { StoryProviders } from "../../../storybook/fixtures";
import { type Look, WashKey } from "./variants";

const meta = {
  title: "Spikes/Wash Key",
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const LOOKS: { look: Look; name: string; note: string }[] = [
  { look: "pill", name: "A · Pill", note: "Today: a filled pill per band." },
  { look: "swatch", name: "B · Swatch", note: "A small block of colour beside a plain word." },
  { look: "square", name: "B2 · Square", note: "The block, square." },
  {
    look: "bar",
    name: "B3 · Bar",
    note: "The block as a short rounded bar, like the wash itself.",
  },
  { look: "stroke", name: "B4 · Stroke", note: "A thin line, like the route line itself." },
  {
    look: "keyed",
    name: "B5 · Keyed",
    note: "The shape says what it keys: a bar for the corridor wash, a stroke for the route line.",
  },
  { look: "underline", name: "C · Underline", note: "Words only, each underlined in its band." },
  { look: "dot", name: "D · Dot", note: "A dot beside the word; the lightest ink." },
  { look: "ramp", name: "E · Ramp", note: "Bands butted into one scale, names inside." },
];

const wind = MEASURES.find((m) => m.key === "wind");
const rain = MEASURES.find((m) => m.key === "rain");

export const SideBySide: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-6 bg-[var(--ground)] p-6">
        {LOOKS.map(({ look, name, note }) => (
          <div key={look} className="grid gap-2">
            <div>
              <h2 className="text-sm font-semibold">{name}</h2>
              <p className="text-xs text-[var(--ink-2)]">{note}</p>
            </div>
            <div className="grid gap-2 rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5">
              {wind ? <WashKey measure={wind} look={look} /> : null}
              {rain ? <WashKey measure={rain} look={look} /> : null}
            </div>
          </div>
        ))}
      </div>
    </StoryProviders>
  ),
};
