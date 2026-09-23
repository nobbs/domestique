import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { riderProfileQuery } from "../../api/queries";
import type { RiderProfile as RiderProfileView } from "../../api/types";
import { WahooDeviceCard } from "./WahooDeviceCard";

function show(view: RiderProfileView) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(riderProfileQuery().queryKey, view);
  render(
    <QueryClientProvider client={client}>
      <WahooDeviceCard />
    </QueryClientProvider>,
  );
}

const zwift = { emailSet: false, passwordSet: false };

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("WahooDeviceCard", () => {
  it("shows set for a credential the rider has entered, unset for the other", () => {
    show({
      profile: {},
      suggestions: {},
      zwift,
      wahoo: { emailSet: true, passwordSet: false, signInRefused: false },
    });

    expect(screen.getByLabelText("Email").getAttribute("placeholder")).toContain("Stored");
    expect(screen.getByLabelText("Password")).toHaveAttribute("placeholder", "Not set");
  });

  it("saves only the fields typed", async () => {
    const view = {
      profile: {},
      suggestions: {},
      zwift,
      wahoo: { emailSet: true, passwordSet: true, signInRefused: false },
    };
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) =>
      init?.method === "GET" || init?.method === undefined
        ? new Response(JSON.stringify(view), { status: 200 })
        : new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show(view);

    await userEvent.type(screen.getByLabelText("Password"), "newpassword");
    await userEvent.click(screen.getByRole("button", { name: "Save Wahoo device sign-in" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const call = fetchMock.mock.calls.find(
      (each) => each[0] === "/v1/settings/rider/credentials/wahoo",
    );
    expect(call?.[1]).toMatchObject({
      method: "PUT",
      body: JSON.stringify({ password: "newpassword" }),
    });
  });

  it("disconnects with a DELETE and no body", async () => {
    const view = {
      profile: {},
      suggestions: {},
      zwift,
      wahoo: { emailSet: true, passwordSet: true, signInRefused: false },
    };
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) =>
      init?.method === "GET" || init?.method === undefined
        ? new Response(JSON.stringify(view), { status: 200 })
        : new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show(view);

    await userEvent.click(screen.getByRole("button", { name: "Disconnect Wahoo device sign-in" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const call = fetchMock.mock.calls.find(
      (each) => each[0] === "/v1/settings/rider/credentials/wahoo",
    );
    expect(call?.[1]).toMatchObject({ method: "DELETE" });
  });

  it("shows the refused alert when Wahoo last refused the stored credentials", () => {
    show({
      profile: {},
      suggestions: {},
      zwift,
      wahoo: { emailSet: true, passwordSet: true, signInRefused: true },
    });

    expect(screen.getByRole("alert")).toHaveTextContent("Wahoo refused this email and password");
  });

  it("shows no refused alert when Wahoo has not refused the stored credentials", () => {
    show({
      profile: {},
      suggestions: {},
      zwift,
      wahoo: { emailSet: true, passwordSet: true, signInRefused: false },
    });

    expect(screen.queryByText(/Wahoo refused/)).not.toBeInTheDocument();
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
        <WahooDeviceCard />
      </QueryClientProvider>,
    );

    expect(
      screen.getByRole("status", { name: "Loading your Wahoo device sign-in" }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        "The service did not say what your Wahoo device sign-in holds.",
      ),
    );
  });
});
