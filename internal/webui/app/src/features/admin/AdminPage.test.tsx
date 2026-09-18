import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { settingsQuery, statusQuery, tasksQuery, webUIConfigQuery } from "../../api/queries";
import { settings, status, tasks } from "../../storybook/fixtures";
import { AdminPage } from "./AdminPage";

function Address() {
  const { pathname } = useLocation();
  return <p data-testid="address">{pathname}</p>;
}

function renderPage(path: string, missing: string[] = []) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ runs: [] }), { status: 200 })),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(settingsQuery().queryKey, { ...settings, missing });
  client.setQueryData(statusQuery().queryKey, status);
  client.setQueryData(tasksQuery().queryKey, tasks);
  client.setQueryData(webUIConfigQuery().queryKey, {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "admin@example.test", admin: true },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Address />
        <Routes>
          <Route path="admin" element={<AdminPage />} />
          <Route path="admin/:section" element={<AdminPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const address = () => screen.getByTestId("address").textContent;

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("AdminPage", () => {
  it("opens on the service tab, holding only that group's settings", () => {
    renderPage("/admin");

    expect(address()).toBe("/admin/service");
    expect(screen.getByRole("tab", { name: "Service" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("Timezone")).toBeInTheDocument();
    expect(screen.queryByText("Wahoo application")).not.toBeInTheDocument();
  });

  it("moves to a tab's own address and shows its group", async () => {
    renderPage("/admin/service");

    await userEvent.click(screen.getByRole("tab", { name: "Integrations" }));

    expect(address()).toBe("/admin/integrations");
    expect(screen.getByText("Wahoo application")).toBeInTheDocument();
    expect(screen.queryByText("Timezone")).not.toBeInTheDocument();
  });

  it("holds the background tasks at /admin/tasks", () => {
    renderPage("/admin/tasks");

    expect(screen.getByRole("heading", { name: "Background tasks" })).toBeInTheDocument();
  });

  // What setup still lacks matters whichever tab is open.
  it("names what is missing above the tabs, on every tab", () => {
    renderPage("/admin/tasks", ["Wahoo client secret"]);

    expect(screen.getByText("Not finished configuring")).toBeInTheDocument();
  });
});
