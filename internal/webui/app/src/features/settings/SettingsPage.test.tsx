import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Status, TargetStatus, WebUIConfig } from "../../api/types";
import { SettingsPage } from "./SettingsPage";

function target(overrides: Partial<TargetStatus> = {}): TargetStatus {
  return {
    id: "rider-a",
    authorisation: "authorized",
    convergence: "current",
    routes: { current: 4, pending: 0 },
    ...overrides,
  };
}

function status(targets: TargetStatus[]): Status {
  return {
    ready: true,
    converged: true,
    targets,
    sync: {
      state: "idle",
      sourceRoutes: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0, enrichmentFailures: 0 },
    },
  };
}

function config(admin = false): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    identity: { display: "rider@example.test", admin },
  };
}

function renderPage(statusValue: Status, configValue: WebUIConfig = config()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(statusQuery().queryKey, statusValue);
  client.setQueryData(webUIConfigQuery().queryKey, configValue);

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <SettingsPage themeChoice="system" onThemeChoiceChange={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("SettingsPage", () => {
  it("shows the caller's own connected target", () => {
    renderPage(status([target()]));

    expect(screen.getByText("rider-a")).toBeInTheDocument();
  });

  it("offers the connect flow when the caller has no target yet", () => {
    renderPage(status([]));

    expect(screen.getByRole("link", { name: "Connect it" })).toBeInTheDocument();
  });

  // The shared service cards moved to the admin page; this page holds only
  // what a rider sees, so neither the settings form nor the tasks link
  // belongs here any more.
  it("does not render the shared service settings or a tasks link", () => {
    renderPage(status([target()]));

    expect(screen.queryByText("Wahoo application")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Open tasks" })).not.toBeInTheDocument();
  });
});
