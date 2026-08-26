import { useQuery } from "@tanstack/react-query";
import { statusQuery } from "../../api/queries";

const REPOSITORY_URL = "https://github.com/nobbs/domestique";

/** How much of a commit a person can read at a glance and still recognise. */
const SHORT_REVISION_LENGTH = 7;

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
export function BuildLine() {
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
