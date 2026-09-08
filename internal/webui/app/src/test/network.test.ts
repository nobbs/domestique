import { describe, expect, it } from "vitest";
import { refusingFetch } from "./network";

describe("refusingFetch", () => {
  it("names what it was asked for, however the request was addressed", async () => {
    const recorded: string[] = [];
    const fetching = refusingFetch(recorded);

    await expect(fetching("/v1/status")).rejects.toThrow();
    await expect(fetching(new URL("https://tiles.example/style.json"))).rejects.toThrow();
    await expect(fetching(new Request("https://tiles.example/tile.pbf"))).rejects.toThrow();

    expect(recorded).toEqual([
      "/v1/status",
      "https://tiles.example/style.json",
      "https://tiles.example/tile.pbf",
    ]);
  });
});
