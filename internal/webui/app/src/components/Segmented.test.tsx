import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Segmented } from "./Segmented";

const ITEMS = [
  { key: "outdoor", label: "Outdoor", fill: "var(--ground-outdoor)" },
  { key: "plain", label: "Plain" },
] as const;

describe("Segmented", () => {
  // jsdom lays nothing out, so the thumb itself is never measured here; its label is.
  it("whitens only a chosen label that sits over a painted thumb", () => {
    const { rerender } = render(
      <Segmented label="Pick" items={ITEMS} value="outdoor" onChange={() => {}} />,
    );
    expect(screen.getByRole("button", { name: "Outdoor" })).toHaveStyle({
      color: "rgb(255, 255, 255)",
    });

    rerender(<Segmented label="Pick" items={ITEMS} value="plain" onChange={() => {}} />);
    expect(screen.getByRole("button", { name: "Outdoor" })).not.toHaveStyle({
      color: "rgb(255, 255, 255)",
    });
    expect(screen.getByRole("button", { name: "Plain" })).not.toHaveStyle({
      color: "rgb(255, 255, 255)",
    });
  });
});
