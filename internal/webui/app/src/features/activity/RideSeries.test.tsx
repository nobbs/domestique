/**
 * The chips that turn a ride's recorded series on, and what each one says.
 *
 * The reading is the point of the chip: with the cursor somewhere on the ride,
 * a chip that only named its series would leave the rider reading values off a
 * line by eye.
 */

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ActivitySeriesName } from "../../api/types";
import type { AlignedSeries } from "../../lib/rideSeries";
import { SeriesChips, type SeriesState } from "./RideSeries";

/** Every series off, which is where a ride page starts. */
function allOff(): Record<ActivitySeriesName, SeriesState> {
  return { heartRate: "off", cadence: "off", speed: "off", temperature: "off", power: "off" };
}

function heartRate(values: (number | null)[]): AlignedSeries {
  return {
    key: "heartRate",
    label: "Heart rate",
    unit: "bpm",
    colour: "var(--series-heart-rate)",
    values,
  };
}

describe("the ride's series chips", () => {
  it("offers every series and asks for the one that is pressed", async () => {
    const onToggle = vi.fn();
    render(<SeriesChips states={allOff()} drawn={[]} activeIndex={null} onToggle={onToggle} />);

    expect(screen.getAllByRole("button")).toHaveLength(5);
    await userEvent.click(screen.getByRole("button", { name: /Heart rate/ }));

    expect(onToggle).toHaveBeenCalledWith("heartRate");
  });

  it("reads the series at the shared cursor", () => {
    render(
      <SeriesChips
        states={{ ...allOff(), heartRate: "drawn" }}
        drawn={[heartRate([120, 148])]}
        activeIndex={1}
        onToggle={vi.fn()}
      />,
    );

    const chip = screen.getByRole("button", { name: /Heart rate/ });
    expect(chip).toHaveAttribute("aria-pressed", "true");
    expect(chip.textContent).toContain("148 bpm");
  });

  // A gap under the cursor says nothing rather than reading as a zero.
  it("says nothing where the series has a gap", () => {
    render(
      <SeriesChips
        states={{ ...allOff(), heartRate: "drawn" }}
        drawn={[heartRate([120, null])]}
        activeIndex={1}
        onToggle={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: /Heart rate/ }).textContent).not.toMatch(/\d/);
  });

  // A bicycle with no meter is answered plainly, not with an error the rider
  // has to interpret.
  it("says a series the ride never recorded is not there", () => {
    render(
      <SeriesChips
        states={{ ...allOff(), power: "absent" }}
        drawn={[]}
        activeIndex={null}
        onToggle={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: /Power/ }).textContent).toContain("not recorded");
    // Still pressed: the rider asked for it, and pressing it again is what
    // puts it away.
    expect(screen.getByRole("button", { name: /Power/ })).toHaveAttribute("aria-pressed", "true");
  });

  // A service that could not be asked says so. Reading a 503 as "not recorded"
  // would tell the rider something false about their own bicycle.
  it("tells a series the ride never recorded from one it could not ask for", () => {
    render(
      <SeriesChips
        states={{ ...allOff(), cadence: "unavailable" }}
        drawn={[]}
        activeIndex={null}
        onToggle={vi.fn()}
      />,
    );

    const chip = screen.getByRole("button", { name: /Cadence/ });
    expect(chip.textContent).toContain("unavailable");
    expect(chip.textContent).not.toContain("not recorded");
  });

  it("marks a series still being fetched as pressed", () => {
    render(
      <SeriesChips
        states={{ ...allOff(), cadence: "loading" }}
        drawn={[]}
        activeIndex={null}
        onToggle={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: /Cadence/ })).toHaveAttribute("aria-pressed", "true");
  });
});
