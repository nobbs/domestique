import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsPage } from "./SettingsPage";

function stubStorage() {
  const entries = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => entries.set(key, value),
  });

  return entries;
}

function show(onThemeChoiceChange = vi.fn()) {
  render(
    <MemoryRouter>
      <SettingsPage themeChoice="system" onThemeChoiceChange={onThemeChoiceChange} />
    </MemoryRouter>,
  );

  return onThemeChoiceChange;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SettingsPage", () => {
  it("keeps the existing unit preference in local storage", async () => {
    const user = userEvent.setup();
    const storage = stubStorage();
    show();

    await user.click(screen.getByRole("radio", { name: "Imperial (mi)" }));

    expect(storage.get("domestique.units")).toBe("imperial");
  });

  it("passes the chosen theme back to the document-level preference", async () => {
    const user = userEvent.setup();
    const onThemeChoiceChange = show();

    await user.click(screen.getByRole("radio", { name: "Dark" }));

    expect(onThemeChoiceChange).toHaveBeenCalledWith("dark");
  });

  it("keeps the map as the return destination", () => {
    stubStorage();
    show();

    expect(screen.getByRole("link", { name: "Back to the map" })).toHaveAttribute("href", "/");
  });
});
