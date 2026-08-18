import { render } from "@testing-library/react";
import { createElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { useEscapeKey } from "./useEscapeKey";

/** A component whose whole job is to hold one registration of the hook. */
function Listener({ active, onEscape }: { active: boolean; onEscape: () => void }) {
  useEscapeKey(active, onEscape);

  return null;
}

function press(key: string): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
  document.dispatchEvent(event);

  return event;
}

describe("useEscapeKey", () => {
  it("hears Escape from wherever the reader is", () => {
    const onEscape = vi.fn();
    render(createElement(Listener, { active: true, onEscape }));

    expect(press("Escape").defaultPrevented).toBe(true);
    expect(onEscape).toHaveBeenCalledTimes(1);
  });

  it("says nothing while there is nothing to leave", () => {
    const onEscape = vi.fn();
    render(createElement(Listener, { active: false, onEscape }));

    expect(press("Escape").defaultPrevented).toBe(false);
    expect(onEscape).not.toHaveBeenCalled();
  });

  it("leaves other keys alone", () => {
    const onEscape = vi.fn();
    render(createElement(Listener, { active: true, onEscape }));

    press("Enter");

    expect(onEscape).not.toHaveBeenCalled();
  });

  // The chart and the map both offer the way back out of a stretch, and both are
  // on the page together whenever the overview is open.
  it("answers one press once, however many are listening", () => {
    const first = vi.fn();
    const second = vi.fn();
    render(
      createElement(
        "div",
        null,
        createElement(Listener, { active: true, onEscape: first }),
        createElement(Listener, { active: true, onEscape: second }),
      ),
    );

    press("Escape");

    expect(first.mock.calls.length + second.mock.calls.length).toBe(1);
  });

  // A gesture still in progress is closer to what the reader is doing than the
  // view it is being drawn on.
  it("stands down for a handler that has already taken the key", () => {
    const onEscape = vi.fn();
    const claim = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
      }
    };
    document.addEventListener("keydown", claim, true);
    render(createElement(Listener, { active: true, onEscape }));

    press("Escape");
    document.removeEventListener("keydown", claim, true);

    expect(onEscape).not.toHaveBeenCalled();
  });

  it("stops listening once there is nothing to leave", () => {
    const onEscape = vi.fn();
    const { rerender } = render(createElement(Listener, { active: true, onEscape }));

    rerender(createElement(Listener, { active: false, onEscape }));
    press("Escape");

    expect(onEscape).not.toHaveBeenCalled();
  });
});
