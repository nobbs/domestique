import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SyncPage } from "./SyncPage";

const REVISION = "0123456789abcdef0123456789abcdef01234567";
const DIGEST = `sha256:${"cd".repeat(32)}`;

function statusBody(build?: Record<string, unknown>) {
  return {
    ready: true,
    converged: true,
    targets: [
      {
        id: "rider-a",
        authorisation: "authorized",
        convergence: "current",
        stages: { current: 4, pending: 0 },
      },
    ],
    sync: {
      state: "idle",
      last_completed_at: "2026-08-18T06:30:00Z",
      source_stages: 4,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 4, total: 4 },
    },
    ...(build ? { build } : {}),
  };
}

/**
 * Every request the page makes, answered from one place. The page is three
 * cards that each fetch for themselves, so a test that stubbed only one of them
 * would be asserting against a page half of which had failed.
 */
function renderPage(path = "/sync", body: unknown = statusBody()) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.startsWith("/v1/sync/runs")) {
        return new Response(JSON.stringify({ runs: [] }), { status: 200 });
      }

      return new Response(JSON.stringify(body), { status: 200 });
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <SyncPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SyncPage", () => {
  /*
   * Three cards, in the order the questions come. The headings are the page: an
   * operator scanning it should be able to find what is happening now without
   * reading the history, and find the history without reading the accounts.
   */
  it("asks the three questions in order, and offers the way back to the map", async () => {
    renderPage();

    expect(screen.getByRole("heading", { level: 1, name: "Sync" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "← Back to the map" })).toHaveAttribute("href", "/");
    const headings = (await screen.findAllByRole("heading", { level: 2 })).map(
      (heading) => heading.textContent,
    );
    expect(headings).toEqual(["Now", "What the accounts hold", "What has happened"]);
  });

  // A notification carries one opaque reference and lands here. A reference for
  // a run the history no longer holds is the pruning working, and the page says
  // so rather than showing nothing.
  it("reads the run a notification named out of the address", async () => {
    renderPage("/sync?run=aaaaaaaaaaaa");

    expect(
      await screen.findByRole("heading", { name: "That run is no longer kept" }),
    ).toBeInTheDocument();
  });

  // `/sync?run=` names no run. Reading the empty string as a reference would put
  // a card at the top of the page about a run nobody asked after.
  it("ignores a run parameter with nothing in it", async () => {
    renderPage("/sync?run=");

    // Waited on so the assertion below is made against a settled history rather
    // than against a card that had not decided what to say yet.
    expect(await screen.findByText("Nothing has run yet.")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /That run/ })).toBeNull();
  });

  it("addresses the exact commit the running service was built from", async () => {
    renderPage("/sync", statusBody({ revision: REVISION, image_digest: DIGEST }));

    const link = await screen.findByRole("link", { name: "commit 0123456" });
    expect(link).toHaveAttribute("href", `https://github.com/nobbs/domestique/commit/${REVISION}`);
    expect(link.getAttribute("title")).toBe(`Source code at commit ${REVISION} · image ${DIGEST}`);
    // Leaving the Tailnet: a new tab, and no referrer handed to GitHub.
    expect(link).toHaveAttribute("target", "_blank");
    expect(link.getAttribute("rel")).toContain("noreferrer");
  });

  it("says a build carries no commit rather than implying one", async () => {
    renderPage("/sync", statusBody());

    const link = await screen.findByRole("link", { name: "a development build" });
    expect(link).toHaveAttribute("href", "https://github.com/nobbs/domestique");
  });

  /*
   * Not knowing yet is not the same as knowing there is no revision: naming a
   * development build on a deployed service, even for one frame, is the exact
   * wrong answer to the question this line exists to settle.
   */
  it("says nothing about the build until the status answers", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/sync"]}>
          <SyncPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(screen.queryByText(/^Running/)).toBeNull();
  });
});
