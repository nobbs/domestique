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
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MapCredits, type MapCreditsProps } from "./MapCredits";

const SURFACE_CREDIT = "Surface data © OpenStreetMap contributors";

/** What a caller supplies, the fold choice aside: that one is the caller's. */
type CreditProps = Omit<MapCreditsProps, "choice" | "onChoiceChange">;

/**
 * Holds the fold choice, as the map does.
 *
 * The component reports a press rather than remembering it, so a test that
 * presses the button needs somebody to report it to.
 */
function Held(props: Partial<CreditProps>) {
  const [choice, setChoice] = useState<boolean | null>(null);

  return (
    <MapCredits
      styleUrl={undefined}
      extra={SURFACE_CREDIT}
      {...props}
      choice={choice}
      onChoiceChange={setChoice}
    />
  );
}

function provided(children: ReactNode) {
  return (
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      {children}
    </QueryClientProvider>
  );
}

function show(props: Partial<CreditProps> = {}) {
  return render(provided(<Held {...props} />));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("MapCredits", () => {
  it("starts folded until the reader asks for the credit", () => {
    show();

    expect(screen.queryByText(SURFACE_CREDIT)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show the map credit" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.getByRole("button", { name: "Show the map credit" })).toHaveAttribute(
      "data-slot",
      "button",
    );
  });

  it("gives the credit back to a keyboard in one interaction", async () => {
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

  it("keeps the reader's choice after it is unfolded", async () => {
    show();

    await userEvent.click(screen.getByRole("button", { name: "Show the map credit" }));

    expect(screen.getByText(SURFACE_CREDIT)).toBeInTheDocument();
  });

  it("keeps the reader's choice when the map moves the credit into its cluster", async () => {
    /*
     * The map draws the credit where it stands until the map reports having a
     * control cluster, and into that cluster afterwards. React unmounts and
     * remounts it across that switch, so a press made while the map was still
     * loading would be undone by the map finishing — unless the choice is held
     * outside, which is what this asserts.
     */
    function Moving() {
      const [choice, setChoice] = useState<boolean | null>(null);
      const [moved, setMoved] = useState(false);
      const credit = (
        <MapCredits
          styleUrl={undefined}
          extra={SURFACE_CREDIT}
          choice={choice}
          onChoiceChange={setChoice}
        />
      );

      return (
        <>
          <button type="button" onClick={() => setMoved(true)}>
            The map found its cluster
          </button>
          {moved ? <section>{credit}</section> : credit}
        </>
      );
    }
    render(provided(<Moving />));

    await userEvent.click(screen.getByRole("button", { name: "Show the map credit" }));
    expect(screen.getByText(SURFACE_CREDIT)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "The map found its cluster" }));

    expect(screen.getByText(SURFACE_CREDIT)).toBeInTheDocument();
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

    await userEvent.click(screen.getByRole("button", { name: "Show the map credit" }));
    expect(await screen.findByText(`© Tile People · ${SURFACE_CREDIT}`)).toBeInTheDocument();
    // The provider's own markup is read for its words and never rendered.
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("draws nothing at all when there is no credit to give", () => {
    const { container } = show({ extra: undefined });

    expect(container).toBeEmptyDOMElement();
  });
});
