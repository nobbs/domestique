import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useOverlayInsets } from "./overlayInsets";

/** A `ResizeObserver` for jsdom, which has none; nothing here resizes. */
class StillObserver {
  observe() {}
  disconnect() {}
}

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return { left, top, width, height, right: left + width, bottom: top + height } as DOMRect;
}

afterEach(() => {
  document.body.innerHTML = "";
  vi.unstubAllGlobals();
});

describe("useOverlayInsets", () => {
  it("measures an overlay that mounts after the page does", async () => {
    vi.stubGlobal("ResizeObserver", StillObserver);
    const { result } = renderHook(() => useOverlayInsets());
    expect(result.current.left).toBe(0);

    const overlay = document.createElement("div");
    overlay.className = "shell__overlay";
    overlay.getBoundingClientRect = () => rect(0, 0, 1280, 800);
    const panel = document.createElement("aside");
    panel.getBoundingClientRect = () => rect(20, 20, 404, 620);
    overlay.append(panel);
    await act(async () => {
      document.body.append(overlay);
    });

    expect(result.current.left).toBe(424);
  });
});
