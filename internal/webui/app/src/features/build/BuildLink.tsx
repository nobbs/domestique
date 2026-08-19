/**
 * Where this service comes from, and which revision of it is running.
 *
 * The one place a reader can tell a deployed build from a development one, and
 * the only link on the page besides a stage's source route that leaves the
 * Tailnet. When the service names a commit, the link addresses that exact commit
 * rather than the repository's moving head — a page describing a running service
 * should point at the source that produced it, not at whatever landed since.
 *
 * It is opened in a new tab with `noreferrer`, so following it neither loses the
 * page being read nor hands GitHub the private origin it is being read on.
 */

import { useQuery } from "@tanstack/react-query";
import { statusQuery } from "../../api/queries";

const REPOSITORY_URL = "https://github.com/nobbs/domestique";

/** How much of a commit a person can read at a glance and still recognise. */
const SHORT_REVISION_LENGTH = 7;

export function BuildLink() {
  const { data } = useQuery(statusQuery());
  const build = data?.build;

  // No commit means no build stamp was compiled in, which is every local build.
  // The label says "dev" rather than nothing, so an operator reading a page can
  // tell they are looking at a development process instead of assuming the
  // deployed one went quiet about its revision.
  const href = build ? `${REPOSITORY_URL}/commit/${build.revision}` : REPOSITORY_URL;
  const label = build ? build.revision.slice(0, SHORT_REVISION_LENGTH) : "dev";
  const title = build
    ? [`Source code at commit ${build.revision}`, build.imageDigest]
        .filter(Boolean)
        .join(" · image ")
    : "Source code on GitHub — this build carries no revision";

  // The mark and a short revision are what a reader sees; the accessible name
  // says the same thing in a sentence, because "0123456" beside an icon means
  // nothing read aloud on its own.
  const name = build
    ? `Source code at commit ${label} on GitHub`
    : "Source code on GitHub — this build carries no revision";

  return (
    <a
      className="layout__source"
      href={href}
      target="_blank"
      rel="noreferrer"
      title={title}
      aria-label={name}
    >
      <svg viewBox="0 0 16 16" width="18" height="18" aria-hidden="true" focusable="false">
        <path
          fill="currentColor"
          d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.42 7.42 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"
        />
      </svg>
      {/*
       * The revision in text beside the mark, rather than only in the title: a
       * tooltip is not there for a keyboard, a touch screen, or a screenshot
       * pasted into a message asking what is deployed.
       */}
      <span className="layout__revision">{label}</span>
    </a>
  );
}
