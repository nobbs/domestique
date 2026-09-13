import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { LegendTable } from "./LegendTable";

const ROWS = [
  { key: 0, colour: "red", label: "Easy", detail: "below 120", value: "1 h", share: "60%" },
  { key: 1, colour: "blue", label: "Hard", value: "40 min", share: "40%" },
];

describe("LegendTable", () => {
  it("reads one row per key, in order", () => {
    render(<LegendTable rows={ROWS} />);

    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Easybelow 1201 h60%");
    expect(rows[1]).toHaveTextContent("Hard40 min40%");
  });

  it("names the row pointed at, and nothing once the pointer leaves the table", () => {
    const onActive = vi.fn();
    render(<LegendTable rows={ROWS} onActive={onActive} />);

    fireEvent.mouseEnter(screen.getAllByRole("row")[1] as Element);
    expect(onActive).toHaveBeenLastCalledWith(1);
    fireEvent.mouseLeave(screen.getByRole("table"));
    expect(onActive).toHaveBeenLastCalledWith(null);
  });

  it("sets every row but the named one back", () => {
    render(<LegendTable rows={ROWS} active={1} />);

    const [easy, hard] = screen.getAllByRole("row");
    expect(easy).toHaveStyle({ opacity: "0.5" });
    expect(hard).toHaveStyle({ opacity: "1" });
    expect(hard).toHaveAttribute("data-active");
  });
});
