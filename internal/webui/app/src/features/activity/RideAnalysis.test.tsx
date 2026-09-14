import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Activity } from "../../api/types";
import { RideAnalysis } from "./RideAnalysis";

const ride: Activity = {
  id: "1",
  startedAt: "2026-09-01T06:00:00Z",
  distanceMetres: 36000,
  movingSeconds: 3600,
  elapsedSeconds: 4000,
  ascentMetres: 420,
  typeId: 0,
  locationId: 0,
  provider: "wahoo",
};

describe("RideAnalysis", () => {
  it("shows the text as written, with the model that wrote it", () => {
    render(
      <RideAnalysis
        ride={{
          ...ride,
          analysis: {
            text: "A steady endurance ride.\n\nKeep tomorrow easy.",
            model: "claude-sonnet-5",
            promptRevision: 1,
            analysedAt: "2026-09-01T08:00:00Z",
          },
        }}
      />,
    );

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("A steady endurance ride.");
    expect(section).toHaveTextContent("Keep tomorrow easy.");
    expect(section).toHaveTextContent("claude-sonnet-5");
  });

  it("shows nothing for a ride not analysed", () => {
    const { container } = render(<RideAnalysis ride={ride} />);

    expect(container).toBeEmptyDOMElement();
  });
});
