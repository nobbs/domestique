import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { refusalMessage, SignInPage } from "./SignInPage";

describe("refusalMessage", () => {
  it("says nothing where nothing failed", () => {
    expect(refusalMessage(null)).toBeNull();
  });

  // The one refusal told apart from the rest: an account that will never be
  // admitted is not a service that is failing.
  it("tells a refused account apart from a failed attempt", () => {
    expect(refusalMessage("not_allowed")).toBe("This account is not allowed to sign in.");
    expect(refusalMessage("failed")).toBe("Sign-in could not be completed.");
  });

  // The reason travels in an address anyone can type, so an unknown one says
  // what every other failure says rather than nothing at all.
  it("reads an unknown reason as a failure", () => {
    expect(refusalMessage("something-else")).toBe("Sign-in could not be completed.");
  });
});

describe("SignInPage", () => {
  function renderAt(entry: string) {
    render(
      <MemoryRouter initialEntries={[entry]}>
        <SignInPage />
      </MemoryRouter>,
    );
  }

  it("submits the document to the route that mints a sign-in", () => {
    renderAt("/auth/login");

    const button = screen.getByRole("button", { name: "Sign in" });
    expect(button).toHaveAttribute("type", "submit");
    expect(button.closest("form")).toHaveAttribute("action", "/auth/start");
    expect(button.closest("form")).toHaveAttribute("method", "post");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // The refusal is drawn from the address the service redirected to, and the
  // control it sits above stays offered: the reader's next move is to retry.
  it("draws the refusal the address carries", () => {
    renderAt("/auth/login?error=not_allowed");

    expect(screen.getByRole("alert")).toHaveTextContent("This account is not allowed to sign in.");
    expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
  });
});
