/**
 * Account: what this rider syncs and how it is going, the accounts it is read
 * from and written to, the numbers their training figures are worked out from,
 * and the sources this service credits. Each is a tab at `/account/{section}`.
 */

import {
  IconBook,
  IconHistory,
  IconPlayerPlay,
  IconRefresh,
  IconTarget,
  IconUser,
  IconUserCircle,
} from "@tabler/icons-react";
import { useSearchParams } from "react-router";
import { PageShell } from "../../components/Layout";
import { type PageTab, PageTabs } from "../../components/PageTabs";
import { DataSources } from "../settings/DataSources";
import { RiderProfile } from "../settings/RiderProfile";
import { WahooAccountCard } from "../settings/WahooAccountCard";
import { ZwiftAccountCard } from "../settings/ZwiftAccountCard";
import { BuildLine } from "../sync/BuildLine";
import { RunNotice } from "../sync/RunNotice";
import { SyncCard } from "../sync/SyncCard";
import { SyncControls } from "../sync/SyncControls";
import { SyncHistory } from "../sync/SyncHistory";
import { TargetConvergenceCard } from "../sync/TargetConvergenceCard";

const icon = (Glyph: typeof IconUser) => <Glyph size={15} stroke={1.8} aria-hidden="true" />;

function SyncTab() {
  const [params] = useSearchParams();
  // The opaque name a Pushover message carries, and nothing else from the query
  // string. A `?run=` with nothing after it names no run.
  const reference = params.get("run") || null;

  return (
    <>
      <RunNotice reference={reference} />
      <SyncCard id="now" heading="Now" icon={<IconPlayerPlay size={18} stroke={1.8} />}>
        <SyncControls />
      </SyncCard>
      <SyncCard
        id="targets"
        heading="What the targets hold"
        icon={<IconTarget size={18} stroke={1.8} />}
      >
        <TargetConvergenceCard />
      </SyncCard>
      <SyncCard
        id="history"
        heading="What has happened"
        icon={<IconHistory size={18} stroke={1.8} />}
      >
        <SyncHistory />
      </SyncCard>
    </>
  );
}

const TABS: readonly PageTab[] = [
  { key: "sync", label: "Sync", icon: icon(IconRefresh), content: <SyncTab /> },
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

export function AccountPage() {
  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <h1 className="font-semibold text-2xl tracking-tight">Account</h1>
        <PageTabs label="Account sections" base="/account" tabs={TABS} />
      </div>
    </PageShell>
  );
}
