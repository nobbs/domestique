import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { webUIConfigQuery } from "../api/queries";
import { ImpersonationBanner } from "./ImpersonationBanner";

function renderBanner(impersonating: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: {
      display: "nina@example.test",
      admin: false,
      ...(impersonating ? { impersonating } : {}),
    },
  });

  return render(
    <QueryClientProvider client={client}>
      <ImpersonationBanner />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ImpersonationBanner", () => {
  it("is absent from an ordinary session", () => {
    renderBanner(false);

    expect(screen.queryByRole("status")).toBeNull();
  });

  it("names the rider and stops the impersonation", async () => {
    const calls = vi.fn(() => Promise.resolve(new Response(null, { status: 204 })));
    vi.stubGlobal("fetch", calls);
    renderBanner(true);

    expect(screen.getByRole("status")).toHaveTextContent("Viewing as nina@example.test");
    await userEvent.click(screen.getByRole("button", { name: "Stop" }));

    expect(calls.mock.calls[0]).toEqual(["/auth/impersonate/stop", expect.anything()]);
  });
});
