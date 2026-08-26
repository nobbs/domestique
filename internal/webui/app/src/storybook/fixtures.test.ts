import { describe, expect, it } from "vitest";
import { stubGlobal } from "./fixtures";

describe("stubGlobal", () => {
  it("puts back the descriptor it found, not the value behind it", () => {
    // The shape `localStorage` has on a window: readable, not writable, and
    // answering from a getter rather than holding a value.
    Object.defineProperty(globalThis, "stubProbe", {
      get: () => "live",
      configurable: true,
      enumerable: true,
    });
    const before = Object.getOwnPropertyDescriptor(globalThis, "stubProbe");

    const restore = stubGlobal("stubProbe", "stubbed");
    expect((globalThis as Record<string, unknown>).stubProbe).toBe("stubbed");
    restore();

    const after = Object.getOwnPropertyDescriptor(globalThis, "stubProbe");
    expect(after?.get).toBe(before?.get);
    expect(after?.enumerable).toBe(true);
    // A value-based undo would leave a writable data property here instead.
    expect(after).not.toHaveProperty("writable");

    Reflect.deleteProperty(globalThis, "stubProbe");
  });

  it("removes a name that had no own property, rather than shadowing one", () => {
    expect(Object.hasOwn(globalThis, "stubAbsent")).toBe(false);

    stubGlobal("stubAbsent", "stubbed")();

    expect(Object.hasOwn(globalThis, "stubAbsent")).toBe(false);
  });
});
