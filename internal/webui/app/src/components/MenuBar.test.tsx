import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { webUIConfigQuery } from "../api/queries";
import type { WebUIConfig } from "../api/types";
import { MenuBar } from "./MenuBar";

function config(admin: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    identity: { display: "rider@example.test", admin },
  };
}

function renderBar(admin: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config(admin));

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <MenuBar />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("the Admin link", () => {
  it("is offered to an admin", () => {
    renderBar(true);

    expect(screen.getByRole("link", { name: "Admin" })).toHaveAttribute("href", "/admin");
  });

  it("is not offered to a non-admin", () => {
    renderBar(false);

    expect(screen.queryByRole("link", { name: "Admin" })).not.toBeInTheDocument();
  });
});
