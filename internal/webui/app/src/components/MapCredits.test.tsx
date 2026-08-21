/**
 * The credit, folded and unfolded.
 *
 * The licences oblige the credit to be reachable, so what these assert is the
 * bargain that lets it fold at all: the affordance is a real button with a name
 * that says what it reveals, its state is reported rather than drawn, and the
 * text behind it is still the text the style document declared.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MapCredits, type MapCreditsProps } from "./MapCredits";

const SURFACE_CREDIT = "Surface data © OpenStreetMap contributors";

function show(props: Partial<MapCreditsProps> = {}) {
  return render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MapCredits styleUrl={undefined} extra={SURFACE_CREDIT} {...props} />
    </QueryClientProvider>,
  );
}

/**
 * Answers the width query yes, which the shared setup answers no.
 *
 * The fold follows the room available, so both answers have to be renderable
 * here — the default the shared stub gives is the wide one.
 */
function stubNarrowViewport() {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: query.includes("max-width"),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("MapCredits", () => {
  it("stands open where there is room for the line", () => {
    show();

    expect(screen.getByText(SURFACE_CREDIT)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Hide the map credit" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("folds away where there is not, and says what it is holding", () => {
    stubNarrowViewport();
    show();

    expect(screen.queryByText(SURFACE_CREDIT)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show the map credit" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("gives the credit back to a keyboard in one interaction", async () => {
    stubNarrowViewport();
    show();

    await userEvent.tab();
    expect(screen.getByRole("button", { name: "Show the map credit" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");

    const credit = screen.getByText(SURFACE_CREDIT);
    expect(credit).toBeInTheDocument();
    // The button points at what it just revealed, and only now that it exists.
    expect(screen.getByRole("button", { name: "Hide the map credit" })).toHaveAttribute(
      "aria-controls",
      credit.id,
    );
  });

  it("keeps the reader's choice when the viewport says otherwise", async () => {
    show();

    await userEvent.click(screen.getByRole("button", { name: "Hide the map credit" }));

    expect(screen.queryByText(SURFACE_CREDIT)).not.toBeInTheDocument();
  });

  it("reads the credit out of the style document, as text, beside the surface one", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              sources: {
                basemap: { attribution: '<a href="https://example.test">&copy; Tile People</a>' },
              },
            }),
          ),
      ),
    );
    show({ styleUrl: "https://tiles.example.test/style.json" });

    expect(await screen.findByText(`© Tile People · ${SURFACE_CREDIT}`)).toBeInTheDocument();
    // The provider's own markup is read for its words and never rendered.
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("draws nothing at all when there is no credit to give", () => {
    const { container } = show({ extra: undefined });

    expect(container).toBeEmptyDOMElement();
  });
});
