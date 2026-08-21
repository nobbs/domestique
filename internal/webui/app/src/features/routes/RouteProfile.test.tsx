import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Position } from "../../api/types";
import { buildProfile } from "../../lib/profile";
import { RouteProfile } from "./RouteProfile";

/** A climb from 100 m to 295 m, so both ends of the range are worth reading. */
function climb(): Position[] {
  return Array.from(
    { length: 40 },
    (_, index): Position => [8, 49 + index * 0.001, 100 + index * 5],
  );
}

const PROFILE = buildProfile(climb());

/**
 * Answers the coarse-pointer query yes, which the shared setup answers no.
 *
 * The hint over the plot is the one thing that reads the pointer, and the two
 * answers are two different sentences, so both have to be renderable here.
 */
function stubCoarsePointer() {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: query.includes("pointer: coarse"),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

function show(props: Partial<React.ComponentProps<typeof RouteProfile>> = {}) {
  const onCollapsedChange = vi.fn();
  render(
    <RouteProfile
      profile={PROFILE}
      title="Col du Test"
      ascentMetres={1560}
      surface={null}
      activeMetres={null}
      onActiveChange={() => {}}
      zoomWindow={null}
      onZoomChange={() => {}}
      highlight={null}
      collapsed={false}
      onCollapsedChange={onCollapsedChange}
      {...props}
    />,
  );

  return { onCollapsedChange };
}

/** The line beside the heading, whatever it happens to be saying. */
function summary(): string {
  return document.querySelector(".route-profile__summary")?.textContent ?? "";
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("RouteProfile", () => {
  it("is a section of the card rather than a panel of its own", () => {
    show();

    expect(screen.getByRole("region", { name: "Elevation" })).toContainElement(
      document.querySelector("#elevation-plot"),
    );
  });

  it("says what a mouse can do with the chart", () => {
    show();

    expect(summary()).toBe("100 m–295 m · drag across to look closer");
  });

  // A finger cannot press and drag in one movement without taking the scroll
  // with it, so the gesture is a different one and has to be named differently.
  it("says what a finger can do with the chart instead", () => {
    stubCoarsePointer();

    show();

    expect(summary()).toBe("100 m–295 m · press and hold to look closer");
  });

  it("says which stretch is on show, and the way back out of it", () => {
    show({ zoomWindow: { startMetres: 500, endMetres: 2900 } });

    expect(summary()).toBe("0.5–2.9 km shown · Escape returns");
  });

  // Nothing to plot is not the same as a plot with nothing in it: a route the
  // service has no elevation for should not be offered a hint about scrubbing.
  it("offers no hint at all for a route with no profile", () => {
    show({ profile: null });

    expect(document.querySelector(".route-profile__summary")).toBeNull();
  });

  it("folds to a row still carrying the figures the chart was there to give", () => {
    show({ collapsed: true });

    expect(summary()).toBe("100 m–295 m · 1,560 m up");
    expect(document.querySelector("#elevation-plot")).toBeNull();
  });

  // A flat ride climbs nothing, and "0 m up" is a figure that reads as a
  // measurement failure rather than as a flat ride.
  it("leaves the climbing out of that row when there is none to give", () => {
    show({ collapsed: true, ascentMetres: 0 });

    expect(summary()).toBe("100 m–295 m");
  });

  it("says nothing in that row for a route it has no figures for", () => {
    show({ collapsed: true, profile: null, ascentMetres: 0 });

    expect(document.querySelector(".route-profile__summary")).toBeNull();
  });

  it("carries the words the chevron does not, and points at the plot it folds", async () => {
    const { onCollapsedChange } = show();
    const fold = screen.getByRole("button", { name: "Hide the profile" });

    expect(fold).toHaveAttribute("aria-expanded", "true");
    expect(fold).toHaveAttribute("aria-controls", "elevation-plot");

    await userEvent.click(fold);

    expect(onCollapsedChange).toHaveBeenCalledWith(true);
  });

  // The plot is unmounted when the row is folded, and a control pointing at an
  // element that is not in the document is a reference nobody can follow.
  it("points at nothing once there is no plot to point at", () => {
    show({ collapsed: true });
    const unfold = screen.getByRole("button", { name: "Show the profile" });

    expect(unfold).toHaveAttribute("aria-expanded", "false");
    expect(unfold).not.toHaveAttribute("aria-controls");
  });
});
