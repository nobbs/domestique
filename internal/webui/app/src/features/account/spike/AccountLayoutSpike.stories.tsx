/**
 * Four layouts for Account, the merge of Sync and Settings.
 *
 * Storybook only. Every card is the real one over the story fixtures.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconBook, IconHistory, IconRefresh, IconUser, IconUserCircle } from "@tabler/icons-react";
import { StoryProviders } from "../../../storybook/fixtures";
import { DataSources } from "../../settings/DataSources";
import { RiderProfile } from "../../settings/RiderProfile";
import { WahooAccountCard } from "../../settings/WahooAccountCard";
import { ZwiftAccountCard } from "../../settings/ZwiftAccountCard";
import { BuildLine } from "../../sync/BuildLine";
import { SyncCard } from "../../sync/SyncCard";
import { SyncControls } from "../../sync/SyncControls";
import { SyncHistory } from "../../sync/SyncHistory";
import { TargetConvergenceCard } from "../../sync/TargetConvergenceCard";
import { Group, SectionRail, SpikePage, type SpikeSection, TopTabs } from "./layouts";

const icon = (Glyph: typeof IconUser) => <Glyph size={16} stroke={1.8} />;

const Now = () => (
  <SyncCard id="now" heading="Now">
    <SyncControls />
  </SyncCard>
);
const Targets = () => (
  <SyncCard id="targets" heading="What the targets hold">
    <TargetConvergenceCard />
  </SyncCard>
);
const History = () => (
  <SyncCard id="history" heading="What has happened">
    <SyncHistory />
  </SyncCard>
);

const SECTIONS: SpikeSection[] = [
  {
    key: "sync",
    label: "Sync",
    icon: icon(IconRefresh),
    content: (
      <>
        <Now />
        <Targets />
      </>
    ),
  },
  {
    key: "accounts",
    label: "Accounts",
    icon: icon(IconUserCircle),
    content: (
      <>
        <WahooAccountCard />
        <ZwiftAccountCard />
      </>
    ),
  },
  { key: "profile", label: "Rider profile", icon: icon(IconUser), content: <RiderProfile /> },
  { key: "history", label: "History", icon: icon(IconHistory), content: <History /> },
  {
    key: "sources",
    label: "Data sources",
    icon: icon(IconBook),
    content: (
      <>
        <DataSources />
        <BuildLine />
      </>
    ),
  },
];

const page = (render: () => React.JSX.Element): Story => ({
  render: () => (
    <StoryProviders>
      <SpikePage title="Account">{render()}</SpikePage>
    </StoryProviders>
  ),
});

const meta = {
  title: "Spikes/Account Layout",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A · Section rail: sections in a vertical rail, one shown at a time. */
export const SectionRailLayout = page(() => <SectionRail sections={SECTIONS} />);

/** B · Main column and rail: sync and history in the main column; accounts, profile, sources in a rail. */
export const MainAndRail = page(() => (
  <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_24rem]">
    <div className="flex min-w-0 flex-col gap-5">
      <Now />
      <Targets />
      <History />
    </div>
    <div className="flex min-w-0 flex-col gap-5">
      <WahooAccountCard />
      <ZwiftAccountCard />
      <RiderProfile />
      <DataSources />
      <BuildLine />
    </div>
  </div>
));

/** C · Top tabs: the same sections as tabs under the title. */
export const TopTabsLayout = page(() => <TopTabs sections={SECTIONS} />);

/** D · Account-centric: each connected account carries its own sync; profile and sources follow. */
export const AccountCentric = page(() => (
  <div className="flex max-w-3xl flex-col gap-6">
    <Group title="Wahoo">
      <WahooAccountCard />
      <Now />
      <Targets />
      <History />
    </Group>
    <Group title="Zwift">
      <ZwiftAccountCard />
    </Group>
    <Group title="You">
      <RiderProfile />
    </Group>
    <Group title="About">
      <DataSources />
      <BuildLine />
    </Group>
  </div>
));
