import { describe, expect, it } from "vitest";
import { sourceRoute } from "./sourceRoute";

const BASE = "https://source.example.test";

describe("sourceRoute", () => {
  it("addresses the route by the identifier the stage already carries", () => {
    expect(sourceRoute(BASE, 4212)?.href).toBe("https://source.example.test/user-routes/4212");
  });

  // The base URL is configuration, and an operator may well write the trailing
  // slash. One slash is a path separator; two are a link to nowhere.
  it("does not double the slash on a base URL that ends in one", () => {
    expect(sourceRoute(`${BASE}/`, 7)?.href).toBe("https://source.example.test/user-routes/7");
    expect(sourceRoute(`${BASE}///`, 7)?.href).toBe("https://source.example.test/user-routes/7");
  });

  it("keeps a prefix a provider is hosted under", () => {
    expect(sourceRoute(`${BASE}/planner/`, 7)?.href).toBe(
      "https://source.example.test/planner/user-routes/7",
    );
  });

  // Changing veloplanner.base_url has to change the link and nothing else has
  // to change with it, which is the whole reason the host is not written here.
  it("follows the configured host rather than a host of its own", () => {
    const moved = sourceRoute("https://elsewhere.example.test", 4212);

    expect(moved?.href).toBe("https://elsewhere.example.test/user-routes/4212");
    expect(moved?.host).toBe("elsewhere.example.test");
  });

  // The affordance shows the name and announces the address, so the name has to
  // be a part of the address or the control is calling itself two things.
  it("names the provider without the parts of an address that are not a name", () => {
    expect(sourceRoute("https://www.source.example.test:8443", 7)?.name).toBe(
      "source.example.test",
    );
    expect(sourceRoute("https://www.source.example.test:8443", 7)?.host).toBe(
      "www.source.example.test:8443",
    );
    expect(sourceRoute(BASE, 7)?.name).toBe("source.example.test");
  });

  it("offers nothing at all when the service names no provider", () => {
    expect(sourceRoute(undefined, 4212)).toBeNull();
    expect(sourceRoute("", 4212)).toBeNull();
  });

  // A link that goes nowhere is worse than no link: a reader cannot tell the two
  // apart until they have followed it and lost the page they were reading.
  it("offers nothing for a base URL that cannot carry a link", () => {
    for (const base of [
      "http://source.example.test",
      "source.example.test",
      "/user-routes",
      "::",
    ]) {
      expect(sourceRoute(base, 4212)).toBeNull();
    }
  });

  it("offers nothing for an identifier that is not one", () => {
    for (const routeId of [0, -3, 1.5, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(sourceRoute(BASE, routeId)).toBeNull();
    }
  });

  // The identifier is the whole of what travels. Anything else on the URL would
  // be telling the provider something about the reader that it did not ask for.
  // The service refuses a base URL carrying either, so this is the second lock
  // on the same door: nothing the operator configured travels to the provider
  // beyond the address of the route.
  it("drops a query or fragment a base URL arrives with", () => {
    expect(sourceRoute(`${BASE}?utm_source=domestique`, 7)?.href).toBe(
      "https://source.example.test/user-routes/7",
    );
    expect(sourceRoute(`${BASE}/#somewhere`, 7)?.href).toBe(
      "https://source.example.test/user-routes/7",
    );
  });

  it("carries no query and no fragment", () => {
    const url = new URL(sourceRoute(BASE, 4212)?.href ?? "");

    expect(url.search).toBe("");
    expect(url.hash).toBe("");
  });
});
