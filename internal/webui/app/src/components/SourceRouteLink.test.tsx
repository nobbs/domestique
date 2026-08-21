import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SourceRouteLink } from "./SourceRouteLink";

const BASE = "https://source.example.test";

describe("SourceRouteLink", () => {
  it("opens the source route, named so its purpose is clear without seeing it", () => {
    render(<SourceRouteLink baseUrl={BASE} routeId={4212} />);

    const link = screen.getByRole("link", {
      name: "Open source route 4212 on source.example.test in a new tab",
    });
    expect(link).toHaveAttribute("href", "https://source.example.test/user-routes/4212");
  });

  // A stage is not addressable at the provider, so the affordance must not
  // promise precision the destination cannot keep.
  it("names the route rather than the stage", () => {
    render(<SourceRouteLink baseUrl={BASE} routeId={4212} />);

    expect(screen.getByRole("link").getAttribute("aria-label")).not.toContain("stage");
  });

  // What a reader cannot work out from the row it sits in is where it goes, so
  // that is what the visible label spends its width on.
  it("shows the provider it leads to", () => {
    render(<SourceRouteLink baseUrl={BASE} routeId={4212} />);

    expect(screen.getByRole("link").textContent).toContain("source.example.test");
  });

  // A visible label the spoken name does not contain is an affordance calling
  // itself two things, which a reader speaking to it cannot resolve.
  it("says out loud the name it shows", () => {
    render(<SourceRouteLink baseUrl="https://www.source.example.test:8443" routeId={4212} />);
    const link = screen.getByRole("link");

    expect(link.textContent).toContain("source.example.test");
    expect(link.getAttribute("aria-label")).toContain("source.example.test");
  });

  // Following it neither loses the stage being read nor hands the provider the
  // private origin the operator is reading it on.
  it("leaves in a new tab and hands the provider no referrer", () => {
    render(<SourceRouteLink baseUrl={BASE} routeId={4212} />);
    const link = screen.getByRole("link");

    expect(link).toHaveAttribute("target", "_blank");
    expect(link.getAttribute("rel")).toContain("noreferrer");
  });

  // Keyboard reachable by being a real link, rather than something clickable
  // that a Tab key walks straight past.
  it("is reachable by keyboard as an ordinary link", () => {
    render(<SourceRouteLink baseUrl={BASE} routeId={4212} />);

    expect(screen.getByRole("link")).not.toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("link").tagName).toBe("A");
  });

  it("renders nothing at all when the service names no provider", () => {
    const { container } = render(<SourceRouteLink baseUrl={undefined} routeId={4212} />);

    expect(screen.queryByRole("link")).toBeNull();
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing rather than a dead link for a base URL it cannot use", () => {
    render(<SourceRouteLink baseUrl="not-a-url" routeId={4212} />);

    expect(screen.queryByRole("link")).toBeNull();
  });
});
