import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { riderProfileQuery } from "../../api/queries";
import type { RiderProfile as RiderProfileView } from "../../api/types";
import { ZwiftAccountCard } from "./ZwiftAccountCard";

function show(view: RiderProfileView) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(riderProfileQuery().queryKey, view);
  render(
    <QueryClientProvider client={client}>
      <ZwiftAccountCard />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ZwiftAccountCard", () => {
  it("shows set for a credential the rider has entered, unset for the other", () => {
    show({ profile: {}, suggestions: {}, zwift: { emailSet: true, passwordSet: false } });

    expect(screen.getByLabelText("Zwift email").getAttribute("placeholder")).toContain("Set");
    expect(screen.getByLabelText("Zwift password")).toHaveAttribute("placeholder", "");
  });

  it("mentions that Zwift's API is unofficial and the rider's own account terms apply", () => {
    show({ profile: {}, suggestions: {}, zwift: { emailSet: false, passwordSet: false } });

    expect(screen.getByText(/unofficial/)).toBeInTheDocument();
  });

  // A save sends only the fields typed, never a value already stored: none is
  // ever sent back to this page to send.
  it("saves only the fields typed", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({}), { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show({ profile: {}, suggestions: {}, zwift: { emailSet: true, passwordSet: true } });

    await userEvent.type(screen.getByLabelText("Zwift password"), "newpassword");
    await userEvent.click(screen.getByRole("button", { name: "Save Zwift account" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const call = fetchMock.mock.calls.find(
      (each) => each[0] === "/v1/settings/rider/credentials/zwift",
    );
    expect(call?.[1]).toMatchObject({
      method: "PUT",
      body: JSON.stringify({ password: "newpassword" }),
    });
  });

  it("disconnects with a DELETE and no body", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({}), { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show({ profile: {}, suggestions: {}, zwift: { emailSet: true, passwordSet: true } });

    await userEvent.click(screen.getByRole("button", { name: "Disconnect Zwift account" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const call = fetchMock.mock.calls.find(
      (each) => each[0] === "/v1/settings/rider/credentials/zwift",
    );
    expect(call?.[1]).toMatchObject({ method: "DELETE" });
  });

  it("says so when the service did not answer the read", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_input: RequestInfo | URL, _init?: RequestInit) =>
          new Response("{}", { status: 503 }),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    render(
      <QueryClientProvider client={client}>
        <ZwiftAccountCard />
      </QueryClientProvider>,
    );

    expect(screen.getByRole("status", { name: "Loading your Zwift account" })).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        "The service did not say what your Zwift account holds.",
      ),
    );
  });
});
