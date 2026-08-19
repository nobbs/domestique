import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BuildLink } from "./BuildLink";

const REVISION = "0123456789abcdef0123456789abcdef01234567";
const DIGEST = `sha256:${"cd".repeat(32)}`;

function statusBody(build?: Record<string, unknown>) {
  return {
    ready: true,
    targets: [],
    sync: {
      state: "idle",
      source_stages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 0, total: 0 },
    },
    ...(build ? { build } : {}),
  };
}

function renderLink(body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(body), { status: 200 })),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return render(
    <QueryClientProvider client={client}>
      <BuildLink />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("BuildLink", () => {
  it("addresses the exact commit the running service was built from", async () => {
    renderLink(statusBody({ revision: REVISION, image_digest: DIGEST }));

    const link = await screen.findByRole("link", { name: /0123456/ });
    // The commit, not the repository head: the page describes a running service,
    // so it must point at the source that produced it and not at what landed
    // since.
    expect(link).toHaveAttribute("href", `https://github.com/nobbs/domestique/commit/${REVISION}`);
  });

  it("shows the revision as text rather than only in a tooltip", async () => {
    renderLink(statusBody({ revision: REVISION }));

    // A title is not there for a keyboard, a touch screen, or a screenshot
    // pasted into a message asking what is deployed.
    expect(await screen.findByText("0123456")).toBeInTheDocument();
  });

  it("names the running image in full where a reader can copy it", async () => {
    renderLink(statusBody({ revision: REVISION, image_digest: DIGEST }));

    const link = await screen.findByRole("link", { name: /0123456/ });
    expect(link.getAttribute("title")).toBe(`Source code at commit ${REVISION} · image ${DIGEST}`);
  });

  it("leaves in a new tab and hands GitHub no referrer", async () => {
    renderLink(statusBody({ revision: REVISION }));

    const link = await screen.findByRole("link", { name: /0123456/ });
    expect(link).toHaveAttribute("target", "_blank");
    expect(link.getAttribute("rel")).toContain("noreferrer");
  });

  it("says a build carries no revision rather than implying one", async () => {
    renderLink(statusBody());

    // "dev" and the repository root, because a local build genuinely has no
    // commit to point at — silence here would read as a deployed build that had
    // gone quiet about its revision.
    const link = await screen.findByRole("link", { name: /carries no revision/ });
    expect(link).toHaveAttribute("href", "https://github.com/nobbs/domestique");
    expect(screen.getByText("dev")).toBeInTheDocument();
    expect(link.getAttribute("title")).toContain("carries no revision");
  });

  it("treats a build group with nothing to identify as no build at all", async () => {
    renderLink(statusBody({ image_digest: DIGEST }));

    // A digest alone says which image is running without saying what is in it,
    // and there is no commit to link to.
    const link = await screen.findByRole("link", { name: /carries no revision/ });
    expect(link).toHaveAttribute("href", "https://github.com/nobbs/domestique");
    expect(screen.queryByText(new RegExp(DIGEST))).toBeNull();
  });

  // Not knowing yet is not the same as knowing there is no revision: "dev" on a
  // deployed service, even for one frame, is the exact wrong answer to the
  // question this affordance exists to settle.
  it("says nothing about the build until the status answers", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <BuildLink />
      </QueryClientProvider>,
    );

    expect(screen.queryByText("dev")).toBeNull();
    expect(screen.getByRole("link").getAttribute("title")).toBe("Source code on GitHub");
  });

  it("still offers the repository while the status is unknown", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("nope", { status: 500 })),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <BuildLink />
      </QueryClientProvider>,
    );

    expect(screen.getByRole("link")).toHaveAttribute("href", "https://github.com/nobbs/domestique");
    // And claims nothing about the build, because a failed fetch says nothing
    // about which one is running.
    expect(screen.queryByText("dev")).toBeNull();
  });
});
