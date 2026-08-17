import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import { ElevationProfile } from "./ElevationProfile";

function climb(): Position[] {
  return Array.from(
    { length: 40 },
    (_, index): Position => [8, 49 + index * 0.001, 100 + index * 5],
  );
}

describe("ElevationProfile", () => {
  it("describes the profile for a reader who cannot see it", () => {
    render(<ElevationProfile coordinates={climb()} title="Eich Rundkurs 90" />);

    const figure = screen.getByRole("img", { name: /Eich Rundkurs 90/ });
    expect(figure).toHaveAccessibleName(/kilometres/);
    expect(figure).toHaveAccessibleName(/metres above sea level/);
  });

  it("exposes scrubbing as a slider so it works by keyboard", () => {
    render(<ElevationProfile coordinates={climb()} title="Eich Rundkurs 90" />);

    const scrub = screen.getByRole("slider", { name: /Position along Eich/ });
    expect(scrub).toHaveAttribute("tabindex", "0");
    expect(scrub).toHaveAttribute("aria-valuemin", "0");
    expect(Number(scrub.getAttribute("aria-valuemax"))).toBeGreaterThan(0);
  });

  it("announces the value under the cursor when stepped by keyboard", async () => {
    const user = userEvent.setup();
    render(<ElevationProfile coordinates={climb()} title="Eich Rundkurs 90" />);

    const scrub = screen.getByRole("slider", { name: /Position along Eich/ });
    await user.tab();
    await user.keyboard("{ArrowRight}");

    expect(scrub.getAttribute("aria-valuetext")).toMatch(/metres at .* kilometres/);
  });

  it("says so plainly when a route has no elevation", () => {
    const flat: Position[] = [
      [8, 49],
      [8, 49.01],
    ];
    render(<ElevationProfile coordinates={flat} title="No profile" />);

    expect(screen.getByText(/no elevation data/i)).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  it("shows the elevation range in the readout before any interaction", () => {
    render(<ElevationProfile coordinates={climb()} title="Eich Rundkurs 90" />);

    // 100 m to 295 m over the generated climb.
    expect(screen.getByText(/100–295 m/)).toBeInTheDocument();
  });
});
