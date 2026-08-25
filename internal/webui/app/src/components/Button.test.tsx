import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { Button, ButtonLink } from "./Button";

describe("Button", () => {
  // A shared control that could quietly become a submit button is a trap, so
  // the type is not among the props a call site can pass.
  it("is always an ordinary button", () => {
    render(<Button>Run now</Button>);

    expect(screen.getByRole("button", { name: "Run now" })).toHaveAttribute("type", "button");
  });

  it("distinguishes the primary action from a standard one", () => {
    render(
      <>
        <Button variant="primary">Open route</Button>
        <Button>Reprocess</Button>
      </>,
    );
    const primary = screen.getByRole("button", { name: "Open route" });
    const standard = screen.getByRole("button", { name: "Reprocess" });

    expect(primary).toHaveClass("bg-primary");
    expect(standard).toHaveClass("bg-background");
  });

  // A navigation that looks like an action is still a link: middle-click, copy
  // the address, and open in a new tab all have to keep working.
  it("renders a link when the action goes somewhere", () => {
    render(
      <MemoryRouter>
        <ButtonLink variant="primary" to="/routes/12/2">
          Open route
        </ButtonLink>
        <Button variant="primary">Run now</Button>
      </MemoryRouter>,
    );

    const link = screen.getByRole("link", { name: "Open route" });
    expect(link).toHaveAttribute("href", "/routes/12/2");
    // The appearance is shared with the button of the same weight, and only the
    // appearance: the element is what makes it a link.
    expect(link.className).toBe(screen.getByRole("button", { name: "Run now" }).className);
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
