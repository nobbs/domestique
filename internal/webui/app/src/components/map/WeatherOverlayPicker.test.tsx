import { IconTemperature, IconWind } from "@tabler/icons-react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WeatherOverlayPicker } from "./WeatherOverlayPicker";

const MEASURES = [
  { key: "wind" as const, label: "Wind", icon: IconWind },
  { key: "temperature" as const, label: "Temperature", icon: IconTemperature },
];

let client: QueryClient;

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("WeatherOverlayPicker", () => {
  beforeEach(() => {
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  });

  it("opens to show a checkbox per measure and toggles it", () => {
    const onToggle = vi.fn();
    render(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set()}
        onToggle={onToggle}
        hoursAhead={0}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "Wind" }));
    expect(onToggle).toHaveBeenCalledWith("wind", true);
  });

  it("shows the hour scale whether or not a measure is on", () => {
    const { rerender } = render(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set()}
        onToggle={vi.fn()}
        hoursAhead={0}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
      { wrapper },
    );
    // Only the prefix this component itself writes, never the formatted
    // weekday and time after it: `toLocaleTimeString` renders those however
    // the runtime's own locale does, which is not this component's to assert.
    expect(screen.getByText(/^Now · /)).toBeInTheDocument();
    // Screen-reader access, not just sight: a slider whose thumb carries no
    // name of its own announces as "slider", unlabelled, on every platform
    // that does not happen to render the sighted layout beside it.
    expect(screen.getByRole("slider", { name: /^When, /u })).toBeInTheDocument();

    rerender(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set(["wind"])}
        onToggle={vi.fn()}
        hoursAhead={3}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
    );
    expect(screen.getByText(/^\+3h · /)).toBeInTheDocument();
  });

  it("asks for the weekday in the hour label, whatever script or order the runtime renders it in", () => {
    // The label's own promise is that a reader scrubbed past midnight can
    // still tell which day they are looking at — a promise the DOM text
    // cannot check without assuming a locale, so this checks the request
    // the component makes instead of the locale-dependent string it gets back.
    const spy = vi.spyOn(Date.prototype, "toLocaleTimeString");
    render(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set()}
        onToggle={vi.fn()}
        hoursAhead={0}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
      { wrapper },
    );

    expect(spy).toHaveBeenCalledWith(undefined, expect.objectContaining({ weekday: "short" }));
    spy.mockRestore();
  });

  it("disables the hour scale until a measure is on", () => {
    const { rerender } = render(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set()}
        onToggle={vi.fn()}
        hoursAhead={0}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.getByRole("slider")).toBeDisabled();

    rerender(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set(["wind"])}
        onToggle={vi.fn()}
        hoursAhead={0}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("slider")).toBeEnabled();
  });

  describe("the hour label over time", () => {
    beforeEach(() => {
      vi.useFakeTimers();
      vi.setSystemTime(new Date("2026-09-05T12:59:00Z"));
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it("re-reads the clock on its own once an hour passes with a measure on", () => {
      const spy = vi.spyOn(Date.prototype, "toLocaleTimeString");
      render(
        <WeatherOverlayPicker
          measures={MEASURES}
          selected={new Set(["wind"])}
          onToggle={vi.fn()}
          hoursAhead={0}
          onHoursAheadChange={vi.fn()}
          expanded={true}
          onExpandedChange={vi.fn()}
        />,
        { wrapper },
      );
      const callsBefore = spy.mock.calls.length;

      act(() => {
        vi.advanceTimersByTime(70 * 60_000);
      });

      expect(spy.mock.calls.length).toBeGreaterThan(callsBefore);
      spy.mockRestore();
    });

    it("does not keep ticking while nothing is on", () => {
      const spy = vi.spyOn(Date.prototype, "toLocaleTimeString");
      render(
        <WeatherOverlayPicker
          measures={MEASURES}
          selected={new Set()}
          onToggle={vi.fn()}
          hoursAhead={0}
          onHoursAheadChange={vi.fn()}
          expanded={true}
          onExpandedChange={vi.fn()}
        />,
        { wrapper },
      );
      const callsBefore = spy.mock.calls.length;

      vi.advanceTimersByTime(70 * 60_000);

      expect(spy.mock.calls.length).toBe(callsBefore);
      spy.mockRestore();
    });
  });

  describe("the loading ring", () => {
    function renderPicker() {
      return render(
        <WeatherOverlayPicker
          measures={MEASURES}
          selected={new Set()}
          onToggle={vi.fn()}
          hoursAhead={0}
          onHoursAheadChange={vi.fn()}
          expanded={false}
          onExpandedChange={vi.fn()}
        />,
        { wrapper },
      );
    }

    it("shows a spinning ring while a grid query fetches, and hides it once it settles", async () => {
      const { container } = renderPicker();
      expect(container.querySelector(".animate-spin")).not.toBeInTheDocument();

      let resolve: (value: number) => void = () => {};
      const pending = new Promise<number>((res) => {
        resolve = res;
      });
      void client.fetchQuery({ queryKey: ["wind-grid", 0, null], queryFn: () => pending });
      await vi.waitFor(() => expect(container.querySelector(".animate-spin")).toBeInTheDocument());

      resolve(1);
      await vi.waitFor(() =>
        expect(container.querySelector(".animate-spin")).not.toBeInTheDocument(),
      );
    });

    it("ignores a fetch for a query outside the weather grid", async () => {
      const { container } = renderPicker();
      void client.fetchQuery({
        queryKey: ["route-geometry", 1],
        queryFn: () => new Promise(() => {}),
      });
      await vi.waitFor(() => expect(client.isFetching()).toBeGreaterThan(0));

      expect(container.querySelector(".animate-spin")).not.toBeInTheDocument();
    });

    it("pulses instead of spinning once the reader has asked for less motion", async () => {
      const restore = window.matchMedia;
      window.matchMedia = (query: string) =>
        ({
          matches: query.includes("prefers-reduced-motion"),
          media: query,
          onchange: null,
          addEventListener: () => {},
          removeEventListener: () => {},
          addListener: () => {},
          removeListener: () => {},
          dispatchEvent: () => false,
        }) as MediaQueryList;

      const { container } = renderPicker();
      void client.fetchQuery({
        queryKey: ["wind-grid", 0, null],
        queryFn: () => new Promise(() => {}),
      });
      await vi.waitFor(() => expect(container.querySelector(".animate-pulse")).toBeInTheDocument());
      expect(container.querySelector(".animate-spin")).not.toBeInTheDocument();

      window.matchMedia = restore;
    });
  });
});
