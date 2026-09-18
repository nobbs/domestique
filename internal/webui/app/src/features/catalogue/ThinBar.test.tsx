import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ThinBar } from "./ThinBar";

describe("ThinBar", () => {
  it("names a sliver as under one percent rather than none", () => {
    render(
      <ThinBar
        label="Surface"
        segments={[
          { key: "paved", label: "Paved", colour: "red", share: 0.996 },
          { key: "gravel", label: "Gravel", colour: "blue", share: 0.004 },
        ]}
      />,
    );

    expect(
      screen.getByRole("img", { name: "Surface: Paved >99%, Gravel <1%" }),
    ).toBeInTheDocument();
  });
});
