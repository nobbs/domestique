/**
 * Sync, as a page.
 *
 * It used to be a strip above the library — a badge, a couple of buttons, and a
 * history a reader scrolled past to get to their routes. Nothing about
 * synchronisation is urgent enough to sit over the map every day, and everything
 * about it needs more room than a strip when it does need attention, so it is a
 * page of its own that the wordmark links to and a notification lands on.
 *
 * Visible settings, then three cards in the order the questions come: what is
 * happening now, what the accounts hold, and what has happened. A notice
 * appears above them when an operator arrived from a notification, or when the
 * last run still needs them.
 */

import { IconArrowLeft } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router";
import { statusQuery } from "../../api/queries";
import { PageShell } from "../../components/Layout";
import { THEME_CHOICES, type ThemeChoice } from "../../lib/theme";
import { useUnitSystem } from "../../lib/units";
import { RunNotice } from "./RunNotice";
import { SyncControls } from "./SyncControls";
import { SyncHistory } from "./SyncHistory";
import { TargetConvergence } from "./TargetConvergence";

const REPOSITORY_URL = "https://github.com/nobbs/domestique";

const THEME_LABELS: Record<ThemeChoice, string> = {
  system: "System",
  light: "Light",
  dark: "Dark",
};

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
    <section className="panel sync-card" aria-labelledby={`${id}-heading`}>
      <h2 className="sync-card__heading" id={`${id}-heading`}>
        {heading}
      </h2>
      {children}
    </section>
  );
}

function Settings({
  themeChoice,
  onThemeChoiceChange,
}: {
  themeChoice: ThemeChoice;
  onThemeChoiceChange: (choice: ThemeChoice) => void;
}) {
  const [unitSystem, setUnitSystem] = useUnitSystem();

  return (
    <SyncCard id="settings" heading="Settings">
      <div className="sync-settings">
        <fieldset className="sync-settings__group">
          <legend>Units</legend>
          <label>
            <input
              type="radio"
              name="units"
              checked={unitSystem === "metric"}
              onChange={() => setUnitSystem("metric")}
            />
            Metric (km)
          </label>
          <label>
            <input
              type="radio"
              name="units"
              checked={unitSystem === "imperial"}
              onChange={() => setUnitSystem("imperial")}
            />
            Imperial (mi)
          </label>
        </fieldset>
        <fieldset className="sync-settings__group">
          <legend>Theme</legend>
          {THEME_CHOICES.map((choice) => (
            <label key={choice}>
              <input
                type="radio"
                name="theme"
                checked={themeChoice === choice}
                onChange={() => onThemeChoiceChange(choice)}
              />
              {THEME_LABELS[choice]}
            </label>
          ))}
        </fieldset>
      </div>
    </SyncCard>
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
    <p className="sync-page__build">
      Running{" "}
      <a href={href} target="_blank" rel="noreferrer" title={title}>
        {label}
      </a>
      .
    </p>
  );
}

export function SyncPage({
  themeChoice = "system",
  onThemeChoiceChange = () => {},
}: {
  themeChoice?: ThemeChoice;
  onThemeChoiceChange?: (choice: ThemeChoice) => void;
}) {
  const [params] = useSearchParams();
  // The opaque name a Pushover message carries, and nothing else from the query
  // string. It is matched against the recorded runs and printed as it is. A
  // `?run=` with nothing after it names no run, so it is read as no parameter
  // rather than as a reference the history will never hold.
  const reference = params.get("run") || null;

  return (
    <PageShell>
      <div className="sync-page">
        <header className="sync-page__header">
          <Link className="sync-page__back" to="/">
            <IconArrowLeft size={16} stroke={2} aria-hidden="true" />
            <span>Back to the map</span>
          </Link>
          <h1 className="sync-page__title">Sync</h1>
        </header>
        <Settings themeChoice={themeChoice} onThemeChoiceChange={onThemeChoiceChange} />
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
