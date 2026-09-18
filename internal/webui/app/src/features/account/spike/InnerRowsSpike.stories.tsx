/**
 * Four treatments for the boxed rows inside the Account and Admin cards: sync
 * phases, targets, sync history and task history.
 *
 * Storybook only. The pages are the real ones; each variant restyles every
 * `rounded-lg border` row and its list from a wrapper, so nothing in the
 * components changes until one is picked.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Route, Routes } from "react-router";
import { StoryProviders } from "../../../storybook/fixtures";
import { AdminPage } from "../../admin/AdminPage";
import { AccountPage } from "../AccountPage";

const VARIANTS = {
  /** A · Today: every row its own outlined box. */
  today: "",
  /** B · Hairline ledger: no boxes; rows split by a rule, flush with the card's edge. */
  ledger:
    "[&_ul.grid]:gap-0 [&_li.rounded-lg.border]:rounded-none [&_li.rounded-lg.border]:border-0 [&_li.rounded-lg.border]:border-b [&_li.rounded-lg.border]:border-[var(--rule)] [&_li.rounded-lg.border]:px-0 [&_li.rounded-lg.border]:py-3 [&_li.rounded-lg.border:last-child]:border-b-0 [&_li.rounded-lg.border:first-child]:pt-0",
  /** C · Washed tiles: no outline; each row a soft tile on a muted wash. */
  tiles:
    "[&_ul.grid]:gap-2 [&_li.rounded-lg.border]:rounded-xl [&_li.rounded-lg.border]:border-0 [&_li.rounded-lg.border]:bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] [&_li.rounded-lg.border]:p-3.5",
  /** D · Grouped inset: one muted block per list, its rows split by the card's colour inside it. */
  inset:
    "[&_ul.grid]:gap-0 [&_ul.grid]:overflow-hidden [&_ul.grid]:rounded-xl [&_ul.grid]:bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] [&_li.rounded-lg.border]:rounded-none [&_li.rounded-lg.border]:border-0 [&_li.rounded-lg.border]:border-b-2 [&_li.rounded-lg.border]:border-[var(--panel)] [&_li.rounded-lg.border]:px-3.5 [&_li.rounded-lg.border:last-child]:border-b-0",
} as const;

type Variant = keyof typeof VARIANTS;

function Pages({ variant, path }: { variant: Variant; path: string }) {
  return (
    <StoryProviders path={path}>
      <div className={VARIANTS[variant]}>
        <Routes>
          <Route path="account/:section" element={<AccountPage />} />
          <Route path="admin/:section" element={<AdminPage />} />
        </Routes>
      </div>
    </StoryProviders>
  );
}

const meta = {
  title: "Spikes/Inner Rows",
  component: Pages,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Pages>;

export default meta;
type Story = StoryObj<typeof meta>;

const sync = "/account/sync";
const tasks = "/admin/tasks";

export const A_TodaySync: Story = { args: { variant: "today", path: sync } };
export const B_LedgerSync: Story = { args: { variant: "ledger", path: sync } };
export const C_TilesSync: Story = { args: { variant: "tiles", path: sync } };
export const D_InsetSync: Story = { args: { variant: "inset", path: sync } };
export const A_TodayTasks: Story = { args: { variant: "today", path: tasks } };
export const B_LedgerTasks: Story = { args: { variant: "ledger", path: tasks } };
export const C_TilesTasks: Story = { args: { variant: "tiles", path: tasks } };
export const D_InsetTasks: Story = { args: { variant: "inset", path: tasks } };
