import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { riderProfileQuery } from "../../api/queries";
import type { RiderProfile as RiderProfileView } from "../../api/types";
import { RiderProfile } from "./RiderProfile";

function show(view: RiderProfileView) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(riderProfileQuery().queryKey, view);
  render(
    <QueryClientProvider client={client}>
      <RiderProfile />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("RiderProfile", () => {
  it("fills each box from the rider's stored parameters", () => {
    show({ profile: { maxHeartRateBpm: 188, riderMassKg: 74.5 }, suggestions: {} });

    expect(screen.getByLabelText("Maximum heart rate (bpm)")).toHaveValue(188);
    expect(screen.getByLabelText("Rider mass (kg)")).toHaveValue(74.5);
    expect(screen.getByLabelText("Functional threshold power (W)")).toHaveValue(null);
  });

  // A suggestion is offered beside the field it is about and applied to none of
  // them: nothing uses one until the rider has typed it in and saved it.
  it("offers a suggestion only where the rides carry that sensor", () => {
    show({ profile: {}, suggestions: { maxHeartRateBpm: 183.4 } });

    expect(screen.getByText(/Your rides of the last 90 days suggest 183 bpm/)).toBeInTheDocument();
    expect(screen.queryByText(/suggest \d+ W/)).not.toBeInTheDocument();
    expect(screen.getByLabelText("Maximum heart rate (bpm)")).toHaveValue(null);
  });

  it("sends the whole profile, leaving out a box the rider cleared", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ profile: {}, suggestions: {} }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show({ profile: { maxHeartRateBpm: 188, bikeMassKg: 8.4 }, suggestions: {} });

    await userEvent.clear(screen.getByLabelText("Bike mass (kg)"));
    await userEvent.type(screen.getByLabelText("Rider mass (kg)"), "74.5");
    await userEvent.click(screen.getByRole("button", { name: "Save rider profile" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const call = fetchMock.mock.calls.find((each) => each[0] === "/v1/settings/rider");
    expect(call?.[1]).toMatchObject({
      method: "PUT",
      body: JSON.stringify({ maxHeartRateBpm: 188, riderMassKg: 74.5 }),
    });
  });

  // The save button is the only way to write: this form has six boxes that
  // block implicit submission, so Enter in one of them submits nothing, and the
  // button is disabled while a write is in flight.
  it("does not write on Enter in a field", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ profile: {}, suggestions: {} }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show({ profile: { maxHeartRateBpm: 188 }, suggestions: {} });

    await userEvent.type(screen.getByLabelText("Maximum heart rate (bpm)"), "{Enter}{Enter}");

    expect(fetchMock.mock.calls.filter((each) => each[0] === "/v1/settings/rider")).toHaveLength(0);
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
        <RiderProfile />
      </QueryClientProvider>,
    );

    expect(screen.getByRole("status", { name: "Loading your rider profile" })).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        "The service did not say what your profile holds.",
      ),
    );
  });

  it("says so when the save was refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_input: RequestInfo | URL, _init?: RequestInit) =>
          new Response("{}", { status: 400 }),
      ),
    );
    show({ profile: { maxHeartRateBpm: 188 }, suggestions: {} });

    await userEvent.click(screen.getByRole("button", { name: "Save rider profile" }));

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
  });
});
