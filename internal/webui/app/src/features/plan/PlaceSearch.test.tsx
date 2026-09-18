import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PlaceMatch } from "../../api/types";

const search = vi.hoisted(() => vi.fn());
const camera = vi.hoisted(() => ({ value: undefined as unknown }));

vi.mock("react-map-gl/maplibre", () => ({ useMap: () => ({ current: camera.value }) }));

vi.mock("../../api/generated", () => ({
  getSearchPlacesQueryOptions: (
    params: { query: string },
    options: { query: { select: (response: unknown) => PlaceMatch[] } },
  ) => ({
    queryKey: ["places", params],
    queryFn: () => search(params),
    select: options.query.select,
  }),
}));

import { PlaceSearch } from "./PlaceSearch";

const TURMBERG: PlaceMatch = {
  name: "Turmberg",
  context: "Durlach, Karlsruhe",
  kind: "peak",
  latitude: 49.0006,
  longitude: 8.4868,
};
const TURMBERGBAHN: PlaceMatch = {
  name: "Turmbergbahn",
  kind: "place",
  latitude: 48.9992,
  longitude: 8.4781,
};
const ETTLINGEN: PlaceMatch = {
  name: "Ettlingen",
  context: "Landkreis Karlsruhe, Baden-Württemberg",
  kind: "settlement",
  latitude: 48.9414,
  longitude: 8.4077,
};

/** A camera over a box of the world, recording where it was asked to frame. */
function cameraOver(west: number, south: number, east: number, north: number) {
  const box = { west, south, east, north };
  const bounds = {
    contains: ([longitude, latitude]: [number, number]) =>
      longitude >= box.west &&
      longitude <= box.east &&
      latitude >= box.south &&
      latitude <= box.north,
    extend: ([longitude, latitude]: [number, number]) => {
      box.west = Math.min(box.west, longitude);
      box.east = Math.max(box.east, longitude);
      box.south = Math.min(box.south, latitude);
      box.north = Math.max(box.north, latitude);
    },
  };
  const fitBounds = vi.fn();
  camera.value = {
    getCenter: () => ({ lng: (west + east) / 2, lat: (south + north) / 2 }),
    getBounds: () => bounds,
    fitBounds,
  };

  return { box, fitBounds };
}

function answering(places: Record<string, PlaceMatch[]>) {
  search.mockReset();
  search.mockImplementation(async ({ query }: { query: string }) => ({
    data: { places: places[query.toLowerCase()] ?? [] },
  }));
}

function renderSearch(onAdd = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <PlaceSearch onAdd={onAdd} />
    </QueryClientProvider>,
  );

  return onAdd;
}

/** Past the search's own debounce, with room for a loaded machine. */
const ANSWERED = { timeout: 5000 };

async function openAndType(text: string) {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: /Search places/ }));
  await user.type(screen.getByRole("searchbox", { name: "Search for a place" }), text);

  return user;
}

describe("PlaceSearch", () => {
  beforeEach(() => {
    camera.value = undefined;
  });

  it("adds the highlighted place with Enter", async () => {
    answering({ turm: [TURMBERG, TURMBERGBAHN] });
    const onAdd = renderSearch();

    const user = await openAndType("turm");
    await screen.findByRole("option", { name: /Durlach, Karlsruhe/ }, ANSWERED);
    await user.keyboard("{ArrowDown}{ArrowUp}{Enter}");

    expect(onAdd).toHaveBeenCalledExactlyOnceWith([TURMBERG]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("marks places across searches and adds them in the order chosen", async () => {
    answering({ turm: [TURMBERG, TURMBERGBAHN], ettl: [ETTLINGEN] });
    const onAdd = renderSearch();

    const user = await openAndType("turm");
    await screen.findByRole("option", { name: /Turmbergbahn/ }, ANSWERED);
    await user.keyboard("{ArrowDown}{Shift>}{Enter}{/Shift}");
    expect(screen.getByRole("button", { name: "Unmark Turmbergbahn" })).toBeTruthy();

    await user.type(screen.getByRole("searchbox"), "ettl");
    await screen.findByRole("option", { name: /Ettlingen/ }, ANSWERED);
    await user.click(screen.getByRole("button", { name: "Mark Ettlingen" }));
    await user.click(screen.getByRole("button", { name: "Add 2 places" }));

    expect(onAdd).toHaveBeenCalledExactlyOnceWith([TURMBERGBAHN, ETTLINGEN]);
  });

  it("adds the marked places and the highlighted one together", async () => {
    answering({ turm: [TURMBERG], ettl: [ETTLINGEN] });
    const onAdd = renderSearch();

    const user = await openAndType("turm");
    await screen.findByRole("option", { name: /Turmberg/ }, ANSWERED);
    await user.keyboard("{Shift>}{Enter}{/Shift}");
    await user.type(screen.getByRole("searchbox"), "ettl");
    await screen.findByRole("option", { name: /Ettlingen/ }, ANSWERED);
    expect(screen.getByText("add 2")).toBeTruthy();
    await user.keyboard("{Enter}");

    expect(onAdd).toHaveBeenCalledExactlyOnceWith([TURMBERG, ETTLINGEN]);
  });

  it("unmarks with Backspace on an empty field, and forgets marks when closed", async () => {
    answering({ turm: [TURMBERG] });
    const onAdd = renderSearch();

    const user = await openAndType("turm");
    await screen.findByRole("option", { name: /Turmberg/ }, ANSWERED);
    await user.keyboard("{Shift>}{Enter}{/Shift}{Backspace}");
    expect(screen.queryByRole("button", { name: "Unmark Turmberg" })).toBeNull();

    await user.type(screen.getByRole("searchbox"), "turm");
    await screen.findByRole("option", { name: /Turmberg/ }, ANSWERED);
    await user.keyboard("{Shift>}{Enter}{/Shift}{Escape}");
    await user.click(screen.getByRole("button", { name: /Search places/ }));

    expect(screen.queryByRole("button", { name: "Unmark Turmberg" })).toBeNull();
    expect(onAdd).not.toHaveBeenCalled();
  });

  it("asks nothing until three characters are typed", async () => {
    answering({});
    renderSearch();

    await openAndType("tu");

    expect(await screen.findByText("Keep typing…", {}, ANSWERED)).toBeTruthy();
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(search).not.toHaveBeenCalled();
  });

  it("says when nothing matches, and when the search is unavailable", async () => {
    answering({});
    renderSearch();

    const user = await openAndType("nowhere");
    expect(await screen.findByText("No place by that name.", {}, ANSWERED)).toBeTruthy();

    search.mockRejectedValue(new Error("502"));
    await user.clear(screen.getByRole("searchbox"));
    await user.type(screen.getByRole("searchbox"), "elsewhere");
    expect(
      await screen.findByText("Place search is unavailable just now.", {}, ANSWERED),
    ).toBeTruthy();
  });

  it("opens with Cmd+K and asks with the trimmed query", async () => {
    answering({ turm: [TURMBERG] });
    renderSearch();
    const user = userEvent.setup();

    await user.keyboard("{Meta>}k{/Meta}");
    await user.type(screen.getByRole("searchbox", { name: "Search for a place" }), "  turm ");

    await screen.findByRole("option", { name: /Turmberg/ }, ANSWERED);
    await waitFor(() => expect(search).toHaveBeenCalledWith({ query: "turm" }), ANSWERED);
  });

  it("leans the search towards the map's centre and says how far each place is", async () => {
    cameraOver(8.3, 48.9, 8.5, 49.1);
    answering({ turm: [TURMBERG] });
    renderSearch();

    await openAndType("turm");

    const option = await screen.findByRole("option", { name: /Turmberg/ }, ANSWERED);
    expect(search).toHaveBeenCalledWith({ query: "turm", latitude: 49, longitude: 8.4 });
    expect(option.textContent).toContain("km");
  });

  it("brings a place the map does not show into view, and leaves the camera for one it does", async () => {
    const { box, fitBounds } = cameraOver(8.3, 48.9, 8.5, 49.1);
    answering({ turm: [TURMBERG], far: [{ ...ETTLINGEN, latitude: 52.5 }] });
    renderSearch();

    const user = await openAndType("turm");
    await user.click(await screen.findByRole("option", { name: /Turmberg/ }, ANSWERED));
    expect(fitBounds).not.toHaveBeenCalled();

    await openAndType("far");
    await screen.findByRole("option", { name: /Ettlingen/ }, ANSWERED);
    await user.keyboard("{Enter}");
    expect(fitBounds).toHaveBeenCalledOnce();
    expect(box.north).toBe(52.5);
  });

  it("unmarks from the chip or the row, and adds nothing when nothing is chosen", async () => {
    answering({ turm: [TURMBERG, TURMBERGBAHN] });
    const onAdd = renderSearch();

    const user = await openAndType("turm");
    await screen.findByRole("option", { name: /Turmbergbahn/ }, ANSWERED);
    await user.click(screen.getByRole("button", { name: "Mark Turmberg" }));
    await user.click(screen.getByRole("button", { name: "Unmark Turmberg" }));
    expect(screen.queryByRole("button", { name: "Unmark Turmberg" })).toBeNull();

    await user.type(screen.getByRole("searchbox"), "turm");
    await screen.findByRole("option", { name: /Turmbergbahn/ }, ANSWERED);
    await user.click(screen.getByRole("button", { name: "Mark Turmbergbahn" }));
    await user.type(screen.getByRole("searchbox"), "turm");
    const row = await screen.findByRole("option", { name: /Turmbergbahn/ }, ANSWERED);
    await user.click(within(row).getByRole("button", { name: "Unmark Turmbergbahn" }));
    expect(screen.queryAllByRole("button", { name: "Unmark Turmbergbahn" })).toHaveLength(0);

    await user.clear(screen.getByRole("searchbox"));
    await user.keyboard("{Enter}{Shift>}{Enter}{/Shift}");
    expect(onAdd).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();
  });
});
