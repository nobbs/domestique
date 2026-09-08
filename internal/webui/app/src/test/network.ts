import { vi } from "vitest";

/**
 * A `fetch` that refuses every request and records what it was asked for.
 *
 * No test in this suite may reach a network service, and one that does fails
 * silently: React Query catches the rejection and the component renders its
 * absent state. The record is what `setup.ts` asserts on afterwards, so an
 * unseeded query fails the test that made it.
 */
export function refusingFetch(recorded: string[]): typeof fetch {
  return ((input: RequestInfo | URL): Promise<Response> => {
    recorded.push(
      typeof input === "string" ? input : input instanceof URL ? input.href : input.url,
    );

    return Promise.reject(new Error("the jsdom suite reaches no network service"));
  }) as typeof fetch;
}

/**
 * A `fetch` that never answers, for a test whose subject is a query in flight.
 *
 * Leaving the query unseeded is not enough — the request goes out for real, and
 * the guard above fails the test for it. Rejecting would be a different state:
 * an answer that the data is not there.
 */
export function stubPendingFetch(): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
}
