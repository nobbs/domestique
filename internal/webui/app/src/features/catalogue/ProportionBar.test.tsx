import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ProportionBar } from "./ProportionBar";

describe("ProportionBar", () => {
  it("sizes each segment by its share of the whole", () => {
    render(
      <ProportionBar
        description="Surface"
        segments={[
          { key: "asphalt", label: "Asphalt", colour: "red", share: 0.7 },
          { key: "gravel", label: "Gravel", colour: "blue", share: 0.3 },
        ]}
      />,
    );

    const [asphalt, gravel] = screen.getAllByTitle(/^(Asphalt|Gravel) /);
    expect(asphalt).toHaveStyle({ flexGrow: "0.7" });
    expect(gravel).toHaveStyle({ flexGrow: "0.3" });
  });

  it("labels a segment wide enough to carry one, name and share both", () => {
    render(
      <ProportionBar
        description="Surface"
        segments={[{ key: "asphalt", label: "Asphalt", colour: "red", share: 0.7 }]}
      />,
    );

    expect(screen.getByText("Asphalt 70%")).toBeInTheDocument();
  });

  it("leaves a sliver too small for a label to its colour and its title alone", () => {
    render(
      <ProportionBar
        description="Surface"
        segments={[
          { key: "asphalt", label: "Asphalt", colour: "red", share: 0.997 },
          { key: "gravel", label: "Gravel", colour: "blue", share: 0.003 },
        ]}
      />,
    );

    expect(screen.queryByText(/Gravel/)).not.toBeInTheDocument();
    expect(screen.getByTitle("Gravel <1%")).toBeInTheDocument();
  });

  it("speaks every class and its share through one accessible name", () => {
    render(
      <ProportionBar
        description="Surface"
        segments={[
          { key: "asphalt", label: "Asphalt", colour: "red", share: 0.7 },
          { key: "gravel", label: "Gravel", colour: "blue", share: 0.3 },
        ]}
      />,
    );

    expect(
      screen.getByRole("img", { name: "Surface: Asphalt 70%, Gravel 30%" }),
    ).toBeInTheDocument();
  });
});
