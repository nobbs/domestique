/**
 * Sync, as a page.
 *
 * It used to be a strip above the library — a badge, a couple of buttons, and a
 * history a reader scrolled past to get to their routes. Nothing about
 * synchronisation is urgent enough to sit over the map every day, and everything
 * about it needs more room than a strip when it does need attention, so it is a
 * page of its own that the wordmark links to and a notification lands on.
 *
 * A notice comes first when attention is needed, followed by three cards in
 * the order the questions come: what is happening now, what the accounts
 * hold, and what has happened.
 */

import { IconArrowLeft } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router";
import { statusQuery } from "../../api/queries";
import { PageShell } from "../../components/Layout";
import { Card, CardContent, CardHeader, CardTitle } from "../../components/ui/card";
import { RunNotice } from "./RunNotice";
import { SyncControls } from "./SyncControls";
import { SyncHistory } from "./SyncHistory";
import { TargetConvergence } from "./TargetConvergence";

const REPOSITORY_URL = "https://github.com/nobbs/domestique";

/** How much of a commit a person can read at a glance and still recognise. */
const SHORT_REVISION_LENGTH = 7;

/** One card: a heading and whatever answers it. */
function SyncCard({
  id,
  heading,
  children,
}: {
  id: string;
  heading: string;
  children: React.ReactNode;
}) {
  return (
    <Card
      className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]"
      role="region"
      aria-labelledby={`${id}-heading`}
    >
      <CardHeader className="pb-3">
        <CardTitle id={`${id}-heading`} role="heading" aria-level={2}>
          {heading}
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4">{children}</CardContent>
    </Card>
  );
}

/**
 * Which build is running, at the foot of the page.
 *
 * The one place a reader can tell a deployed build from a development one, and
 * the only link out of the Tailnet besides a route's source. When the service
 * names a commit the link addresses that exact commit rather than the
 * repository's moving head: a page describing a running service should point at
 * the source that produced it, not at whatever landed since.
 *
 * A new tab with `noreferrer`, so following it neither loses the page being read
 * nor hands GitHub the private origin it is being read on.
 */
function BuildLine() {
  const { data, isPending, isError } = useQuery(statusQuery());
  const build = data?.build;
  // Not knowing yet is not the same as knowing there is no revision. Until the
  // status answers — and if it never does — the line says nothing about the
  // build at all, because "dev" on a deployed service, even for one frame, is
  // the exact wrong answer to the question this line exists to settle.
  if (isPending || isError) {
    return null;
  }

  const href = build ? `${REPOSITORY_URL}/commit/${build.revision}` : REPOSITORY_URL;
  // No commit in an answer that did arrive means no build stamp was compiled
  // in, which is every local build. It says "a development build" rather than
  // nothing, so an operator can tell which process they are looking at.
  const label = build
    ? `commit ${build.revision.slice(0, SHORT_REVISION_LENGTH)}`
    : "a development build";
  // The digest names the running image without saying what is in it, so it
  // hangs off the link rather than taking a line of its own.
  const title = build
    ? `Source code at commit ${build.revision}${build.imageDigest ? ` · image ${build.imageDigest}` : ""}`
    : "Source code on GitHub";

  return (
    <p className="text-sm text-[var(--ink-2)]">
      Running{" "}
      <a href={href} target="_blank" rel="noreferrer" title={title}>
        {label}
      </a>
      .
    </p>
  );
}

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
        <header className="flex items-center gap-4">
          <Link
            className="inline-flex items-center gap-1 text-sm text-[var(--ink-2)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
            to="/"
          >
            <IconArrowLeft size={16} stroke={2} aria-hidden="true" />
            <span>Back to the map</span>
          </Link>
          <h1 className="text-2xl font-semibold tracking-tight">Sync</h1>
        </header>
        <RunNotice reference={reference} />
        <SyncCard id="now" heading="Now">
          <SyncControls />
        </SyncCard>
        <SyncCard id="accounts" heading="What the accounts hold">
          <TargetConvergence />
        </SyncCard>
        <SyncCard id="history" heading="What has happened">
          <SyncHistory />
        </SyncCard>
        <BuildLine />
      </div>
    </PageShell>
  );
}
