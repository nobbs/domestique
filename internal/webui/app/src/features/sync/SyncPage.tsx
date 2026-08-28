/**
 * Sync, as a page.
 *
 * It used to be a strip above the library — a badge, a couple of buttons, and a
 * history a reader scrolled past to get to their routes. Nothing about
 * synchronisation is urgent enough to sit over the map every day, and everything
 * about it needs more room than a strip when it does need attention, so it is a
 * page of its own that the menu bar links to and a notification lands on.
 *
 * A notice comes first when attention is needed, followed by three cards in
 * the order the questions come: what is happening now, what the accounts
 * hold, and what has happened.
 */

import { useSearchParams } from "react-router";
import { PageShell } from "../../components/Layout";
import { BuildLine } from "./BuildLine";
import { RunNotice } from "./RunNotice";
import { SyncCard } from "./SyncCard";
import { SyncControls } from "./SyncControls";
import { SyncHistory } from "./SyncHistory";
import { TargetConvergenceCard } from "./TargetConvergenceCard";

export function SyncPage() {
  const [params] = useSearchParams();
  // The opaque name a Pushover message carries, and nothing else from the query
  // string. It is matched against the recorded runs and printed as it is. A
  // `?run=` with nothing after it names no run, so it is read as no parameter
  // rather than as a reference the history will never hold.
  const reference = params.get("run") || null;

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <h1 className="text-2xl font-semibold tracking-tight">Sync</h1>
        <RunNotice reference={reference} />
        <SyncCard id="now" heading="Now">
          <SyncControls />
        </SyncCard>
        <SyncCard id="accounts" heading="What the accounts hold">
          <TargetConvergenceCard />
        </SyncCard>
        <SyncCard id="history" heading="What has happened">
          <SyncHistory />
        </SyncCard>
        <BuildLine />
      </div>
    </PageShell>
  );
}
