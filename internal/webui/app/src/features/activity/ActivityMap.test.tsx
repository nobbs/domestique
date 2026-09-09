/**
 * The ride map's own furniture, without a canvas: the expand toggle it adds
 * beside the zoom pair, which `ActivityPage.test.tsx` mocks away.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { webUIConfigQuery } from "../../api/queries";
import type { WebUIConfig } from "../../api/types";
import { ChromeMap } from "../../storybook/mapMock";
import { ActivityMap } from "./ActivityMap";

// The overlay draws the recorded track itself, which needs a real MapLibre
// instance this test never mounts; only the furniture around it is in question.
vi.mock("../routes/RouteOverlay", () => ({ RouteOverlay: () => null }));

const CONFIG: WebUIConfig = {
  basemaps: [
    { name: "Streets", styleUrl: "https://example.test/style.json", darkCartography: false },
  ],
  sourceBaseUrls: {},
  timezone: "Europe/Berlin",
  identity: { display: "rider@example.test", admin: false },
};

function show(expanded: boolean, onExpandedChange = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, CONFIG);

  render(
    <QueryClientProvider client={client}>
      <ChromeMap>
        <ActivityMap
          coordinates={[]}
          bounds={[8.4, 49, 8.6, 49.2]}
          profile={null}
          activeMetres={null}
          onActiveChange={() => {}}
          expanded={expanded}
          onExpandedChange={onExpandedChange}
        />
      </ChromeMap>
    </QueryClientProvider>,
  );

  return onExpandedChange;
}

describe("ActivityMap's expand toggle", () => {
  it("offers to expand when collapsed", () => {
    show(false);

    expect(screen.getByRole("button", { name: "Expand map" })).toBeInTheDocument();
  });

  it("offers to collapse when expanded, and reports a press", async () => {
    const user = userEvent.setup();
    const onExpandedChange = show(true);

    await user.click(screen.getByRole("button", { name: "Collapse map" }));

    expect(onExpandedChange).toHaveBeenCalledWith(false);
  });
});
