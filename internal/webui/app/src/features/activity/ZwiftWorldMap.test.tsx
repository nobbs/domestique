/**
 * The world map's overlay, which jsdom will never paint: what is asserted is
 * where the line and the cursor were placed, and what a hover reports back.
 */

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ActivityTrackWorld, Position } from "../../api/types";
import { buildActivityProfile } from "../../lib/profile";
import { ZwiftWorldMap } from "./ZwiftWorldMap";

const WORLD: ActivityTrackWorld = {
  id: 9,
  name: "Makuri Islands",
  mapUrl: "/v1/zwift/worlds/9/map",
  bounds: { north: -10.7, west: 165.7, south: -10.9, east: 165.9 },
};

// A line from the north-west corner to the south-east one, so every point of
// it lands somewhere the transform's own test already pins down.
const COORDINATES: Position[] = [
  [165.7, -10.7, 10],
  [165.8, -10.8, 20],
  [165.9, -10.9, 30],
];

const SIZE = { width: 1000, height: 500 };

/** Renders the map with its artwork already loaded at a known pixel size. */
function show(activeMetres: number | null = null, onActiveChange = vi.fn()) {
  render(
    <ZwiftWorldMap
      world={WORLD}
      coordinates={COORDINATES}
      profile={buildActivityProfile(COORDINATES)}
      activeMetres={activeMetres}
      onActiveChange={onActiveChange}
      expanded={false}
      onExpandedChange={() => {}}
    />,
  );
  const artwork = screen.getByRole("img", { name: "Map of Makuri Islands" });
  Object.defineProperty(artwork, "naturalWidth", { value: SIZE.width });
  Object.defineProperty(artwork, "naturalHeight", { value: SIZE.height });
  fireEvent.load(artwork);

  return onActiveChange;
}

describe("ZwiftWorldMap", () => {
  it("draws nothing over the artwork until it knows its size", () => {
    render(
      <ZwiftWorldMap
        world={WORLD}
        coordinates={COORDINATES}
        profile={null}
        activeMetres={null}
        onActiveChange={() => {}}
        expanded={false}
        onExpandedChange={() => {}}
      />,
    );

    expect(screen.queryByLabelText("Recorded track in Makuri Islands")).not.toBeInTheDocument();
  });

  it("places the track across the loaded artwork", () => {
    show();

    const overlay = screen.getByLabelText("Recorded track in Makuri Islands");
    expect(overlay).toHaveAttribute("viewBox", "0 0 1000 500");
    expect(overlay.querySelector("polyline")).toHaveAttribute(
      "points",
      "0.0,0.0 500.0,250.0 1000.0,500.0",
    );
  });

  it("marks the shared cursor position on the artwork", () => {
    show(0);

    const marker = screen
      .getByLabelText("Recorded track in Makuri Islands")
      .querySelector("circle");
    expect(marker).toHaveAttribute("cx", "0");
    expect(marker).toHaveAttribute("cy", "0");
  });

  it("reports the distance nearest a hover", () => {
    const onActiveChange = show();
    const overlay = screen.getByLabelText("Recorded track in Makuri Islands");
    vi.spyOn(overlay, "getBoundingClientRect").mockReturnValue({
      left: 0,
      top: 0,
      width: SIZE.width,
      height: SIZE.height,
    } as DOMRect);

    // The south-east end of the line, so the distance reported is the ride's
    // own length rather than a point part-way along it.
    fireEvent.pointerMove(overlay, { clientX: 1000, clientY: 500 });

    const profile = buildActivityProfile(COORDINATES);
    expect(onActiveChange).toHaveBeenCalledWith(profile?.totalDistanceMetres);
  });

  it("clears the cursor when the pointer leaves", () => {
    const onActiveChange = show();

    fireEvent.pointerLeave(screen.getByLabelText("Recorded track in Makuri Islands"));

    expect(onActiveChange).toHaveBeenCalledWith(null);
  });
});
