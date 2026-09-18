import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getGetPlanDeliveryQueryKey, type PlanTargetDelivery } from "../../api/generated";
import { PlanDeliveryTrigger, pollInterval } from "./PlanDelivery";

afterEach(() => {
  vi.unstubAllGlobals();
});

function target(overrides: Partial<PlanTargetDelivery> = {}): PlanTargetDelivery {
  return { id: "t1", own: false, state: "current", ...overrides };
}

/**
 * Answers the delivery GET this plan's id would produce, and a bare 202 to
 * anything else (the task run POST) — one router for both the cache-seeded
 * initial render and the forced refetch the popover triggers on open.
 */
function stubFetch(targets: PlanTargetDelivery[], planId: number) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.includes(`/v1/plans/${planId}/delivery`)) {
      // The wire body is the bare `PlanDelivery`; `domestiqueRequest` wraps it.
      return new Response(JSON.stringify({ targets }), { status: 200 });
    }

    return new Response(JSON.stringify({}), { status: 202 });
  });
  vi.stubGlobal("fetch", fetchMock);

  return fetchMock;
}

function renderTrigger(targets: PlanTargetDelivery[], published = true, planId = 4) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(getGetPlanDeliveryQueryKey(planId), { data: { targets } });
  const fetchMock = stubFetch(targets, planId);

  return {
    fetchMock,
    ...render(
      <QueryClientProvider client={client}>
        <PlanDeliveryTrigger planId={planId} published={published} />
      </QueryClientProvider>,
    ),
  };
}

function trigger() {
  return screen.getByRole("button", { name: "Wahoo delivery status" });
}

describe("PlanDeliveryTrigger", () => {
  it("renders nothing for an unsaved plan", () => {
    const client = new QueryClient();
    const { container } = render(
      <QueryClientProvider client={client}>
        <PlanDeliveryTrigger planId={null} published={false} />
      </QueryClientProvider>,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("draws a green dot once every rider is current", () => {
    const { container } = renderTrigger([
      target({ state: "current", deliveredAt: "2026-09-18T09:00:00Z" }),
    ]);

    const dot = container.querySelector(".plan-delivery-dot");
    expect(dot).toHaveStyle({ background: "var(--good)" });
    expect(dot).not.toHaveClass("animate-pulse");
  });

  it("draws a pulsing dot while a rider is pending", () => {
    const { container } = renderTrigger([
      target({ id: "t1", state: "current", deliveredAt: "2026-09-18T09:00:00Z" }),
      target({ id: "t2", state: "pending" }),
    ]);

    const dot = container.querySelector(".plan-delivery-dot");
    expect(dot).toHaveStyle({ background: "var(--hold)" });
    expect(dot).toHaveClass("animate-pulse");
  });

  it("draws an alert dot when a rider has failed", () => {
    const { container } = renderTrigger([target({ state: "failed", failure: "authorization" })]);

    expect(container.querySelector(".plan-delivery-dot")).toHaveStyle({
      background: "var(--alert)",
    });
  });

  it("draws no dot for a plan that was never published", () => {
    const { container } = renderTrigger([target({ state: "absent" })], false);

    expect(container.querySelector(".plan-delivery-dot")).toBeNull();
  });

  it("names the rider, marking their own row and falling back for no nickname", async () => {
    renderTrigger([
      target({ id: "t1", own: true, ownerNickname: "Alexej", state: "current" }),
      target({ id: "t2", own: false, state: "current", deliveredAt: "2026-09-18T08:45:00Z" }),
    ]);
    fireEvent.click(trigger());

    expect(await screen.findByText("Alexej (you)")).toBeInTheDocument();
    expect(screen.getByText("Unnamed rider")).toBeInTheDocument();
  });

  it("reads a failure in words, from the sync failure category", async () => {
    renderTrigger([target({ state: "failed", failure: "authorization" })]);
    fireEvent.click(trigger());

    expect(await screen.findByText("Must reconnect Wahoo")).toBeInTheDocument();
  });

  it("reads an unrecognised failure category as a generic push failure", async () => {
    renderTrigger([target({ state: "failed", failure: "something_new" })]);
    fireEvent.click(trigger());

    expect(await screen.findByText("Push failed")).toBeInTheDocument();
  });

  it("reads a pending target as removal once the plan is unpublished", async () => {
    renderTrigger([target({ state: "pending", deliveredAt: "2026-09-16T07:00:00Z" })], false);
    fireEvent.click(trigger());

    expect(await screen.findByText("Removing from 1 rider")).toBeInTheDocument();
    expect(screen.getByText("Removing…")).toBeInTheDocument();
  });

  it("shows the draft message rather than any rows for a never-published plan", async () => {
    renderTrigger([target({ state: "absent" })], false);
    fireEvent.click(trigger());

    expect(
      await screen.findByText(
        "Publishing sends this plan to every connected rider's Wahoo account.",
      ),
    ).toBeInTheDocument();
  });

  it("retries a failed push against the task endpoint, with this plan's id", async () => {
    const { fetchMock } = renderTrigger(
      [target({ state: "failed", failure: "destination" })],
      true,
      7,
    );
    fireEvent.click(trigger());
    fireEvent.click(await screen.findByRole("button", { name: /Retry/ }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/v1/tasks/sync%3Aplan/run/7"),
        expect.anything(),
      ),
    );
  });
});

describe("PlanDeliveryTrigger edge cases", () => {
  it("says a copy a full sync wrote is on Wahoo, without a time it does not know", async () => {
    renderTrigger([target({ ownerNickname: "Marta", state: "current" })]);
    fireEvent.click(trigger());

    await screen.findByText("Marta");
    expect(screen.queryByText(/Delivered/)).toBeNull();
    expect(screen.queryByText(/never/)).toBeNull();
  });

  it("offers no retry where only the rider reconnecting helps", async () => {
    renderTrigger([target({ state: "failed", failure: "authorization" })]);
    fireEvent.click(trigger());

    await screen.findByText("Must reconnect Wahoo");
    expect(screen.queryByRole("button", { name: /Retry/ })).toBeNull();
  });

  it("says no rider is connected rather than calling a published plan a draft", async () => {
    const { container } = renderTrigger([], true);
    fireEvent.click(trigger());

    expect(await screen.findByText("No rider has connected Wahoo yet.")).toBeInTheDocument();
    expect(screen.queryByText("Not published")).toBeNull();
    expect(container.querySelector(".plan-delivery-dot")).toBeNull();
  });
});

describe("pollInterval", () => {
  it("polls only while a target is pending", () => {
    expect(pollInterval([target({ state: "current" })])).toBe(false);
    expect(pollInterval([target({ state: "pending" })])).toBe(2000);
    expect(pollInterval([target({ state: "absent" })])).toBe(false);
    expect(pollInterval([])).toBe(false);
  });
});
