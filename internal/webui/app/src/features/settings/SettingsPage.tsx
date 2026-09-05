import { PageShell } from "../../components/Layout";
import { DataSources } from "./DataSources";
import { WahooAccountCard } from "./WahooAccountCard";

export function SettingsPage() {
  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        {/*
         * The one thing this page holds that is not local, and the one thing
         * here that is this rider's own rather than the whole service's. The
         * colour scheme used to sit above it and now sits in the bar, where it
         * is reachable from every page rather than from this one.
         */}
        <WahooAccountCard />
        {/*
         * Last, because it is reference rather than a setting: nothing here is
         * changed, and every credit this service owes is read from one place.
         */}
        <DataSources />
      </div>
    </PageShell>
  );
}
