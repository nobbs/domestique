import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Button } from "./Button";

/** The base class first, then the variant, as the component composes them. */
function classesOf(element: HTMLElement): string[] {
  return element.className.split(" ");
}

describe("Button", () => {
  // A shared control that could quietly become a submit button is a trap, so
  // the type is not among the props a call site can pass.
  it("is always an ordinary button", () => {
    render(<Button>Run now</Button>);

    expect(screen.getByRole("button", { name: "Run now" })).toHaveAttribute("type", "button");
  });

  it("dresses a quiet action differently from a standard one, on the same base", () => {
    render(
      <>
        <Button>Run now</Button>
        <Button variant="quiet">Reprocess</Button>
      </>,
    );
    const standard = classesOf(screen.getByRole("button", { name: "Run now" }));
    const quiet = classesOf(screen.getByRole("button", { name: "Reprocess" }));

    expect(standard[0]).toBe(quiet[0]);
    expect(standard[1]).not.toBe(quiet[1]);
  });

  it("keeps the class the feature placing it asked for", () => {
    render(<Button className="somewhere-specific">Reprocess</Button>);

    expect(screen.getByRole("button", { name: "Reprocess" })).toHaveClass("somewhere-specific");
  });

  // Disabled here only ever means "that request is already in flight", and the
  // whole point of it is that a second press does not send a second request.
  it("asks for nothing while it is disabled", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <Button disabled onClick={onClick}>
        Requesting…
      </Button>,
    );
    const button = screen.getByRole("button", { name: "Requesting…" });

    expect(button).toBeDisabled();
    await user.click(button);

    expect(onClick).not.toHaveBeenCalled();
  });

  it("says what it does when the visible words do not", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <Button aria-label="Run now: Read from VeloPlanner" onClick={onClick}>
        Run now
      </Button>,
    );

    await user.click(screen.getByRole("button", { name: "Run now: Read from VeloPlanner" }));

    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
