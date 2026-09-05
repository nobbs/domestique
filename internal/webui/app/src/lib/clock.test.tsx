/**
 * The tick, without any of the components that lean on it.
 */

import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useHourTick } from "./clock";

let renders = 0;

function Harness({ on }: { on: boolean }) {
  renders += 1;
  useHourTick(on);

  return null;
}

beforeEach(() => {
  renders = 0;
  vi.useFakeTimers();
  // A fixed moment shy of the hour, so advancing by a little over an hour is
  // guaranteed to cross exactly one boundary.
  vi.setSystemTime(new Date("2026-09-05T12:59:00Z"));
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useHourTick", () => {
  it("re-renders its caller on its own once the clock crosses into a new hour", () => {
    render(<Harness on={true} />);
    const before = renders;

    act(() => {
      vi.advanceTimersByTime(70 * 60_000);
    });

    expect(renders).toBeGreaterThan(before);
  });

  it("schedules nothing while switched off", () => {
    render(<Harness on={false} />);
    const before = renders;

    vi.advanceTimersByTime(70 * 60_000);

    expect(renders).toBe(before);
  });
});
