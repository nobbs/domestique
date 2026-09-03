/**
 * The sign-in and sign-out round trip, through the real gate: the application
 * document at `/auth/login`, the `/auth/start` redirect, and the fake issuer
 * `dev/demoapi` stands up for a demo.
 *
 * Every test here signs in for itself rather than reusing `bundlePage`'s
 * pre-minted cookie: that cookie is one row in the demo's database, shared by
 * every other test in this project, and a test that revoked it would end
 * every other bundle-project test's session out from under it. A fresh
 * interactive sign-in costs one more round trip and leaves the shared session
 * alone.
 */

import type { Page } from "@playwright/test";
import { expect, test } from "./fixtures";

// The fake issuer's certificate is self-signed; the interactive tests below
// navigate to it as part of the flow.
test.use({ ignoreHTTPSErrors: true });

/**
 * Forwards every request to the service or the fake issuer, overriding Origin
 * on the ones that carry one the way the Vite dev proxy and `bundlePage` both
 * do, and refuses everything else rather than letting it reach a real third
 * party — this page has no session yet, so `installOfflineBasemap`'s own
 * setup request would 401.
 *
 * Loopback rather than the service's own origin alone: the flow's middle hop
 * is the fake issuer `dev/demoapi` stands up on its own loopback port, which
 * this test does not otherwise know the address of.
 */
async function offline(page: Page, originHeader: string): Promise<void> {
  await page.route("**/*", async (route) => {
    const request = route.request();
    const url = request.url();
    const loopback = /^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?\//.test(url);
    if (!loopback && !url.startsWith("data:") && !url.startsWith("blob:")) {
      await route.abort("blockedbyclient");

      return;
    }
    if (!("origin" in request.headers())) {
      await route.continue();

      return;
    }
    // maxRedirects: 0 to read the sign-in cookie `/auth/start` sets without
    // this fetch silently walking the rest of the chain by itself.
    const response = await route.fetch({
      headers: { ...request.headers(), origin: originHeader },
      maxRedirects: 0,
    });
    const location = response.headers().location;
    if (response.status() < 300 || response.status() >= 400 || !location) {
      await route.fulfill({ response });

      return;
    }
    // Chromium does not follow a redirect status handed back through
    // route.fulfill for a navigation request, so the hop is done as an
    // ordinary page the browser then navigates itself — a real top-level
    // navigation, which is what lets it process the Set-Cookie above and
    // send the browser on to the next hop over the real network.
    const setCookie = response.headers()["set-cookie"];
    await route.fulfill({
      status: 200,
      contentType: "text/html",
      headers: setCookie === undefined ? {} : { "set-cookie": setCookie },
      body: `<!doctype html><meta http-equiv="refresh" content="0;url=${location}">`,
    });
  });
}

/** Signs in through the real flow and leaves the page on "/", signed in. */
async function signIn(page: Page, originHeader: string): Promise<void> {
  await offline(page, originHeader);
  await page.goto("/auth/login");
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL((url) => url.pathname === "/");
}

test("an unauthenticated page request is sent to sign in", async ({ page }) => {
  await page.goto("/");

  await expect(page).toHaveURL(/\/auth\/login$/);
  await expect(page.getByRole("button", { name: "Sign in" })).toHaveCount(1);
});

test("signing in completes the round trip and boots the app", async ({ page, identity }) => {
  await signIn(page, identity.origin ?? "");

  await expect(page.getByRole("button", { name: "Search the route library" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Signed in as rider@example.test" })).toBeVisible();
});

test("signing out ends the session and the next page request is sent back to sign in", async ({
  page,
  identity,
}) => {
  await signIn(page, identity.origin ?? "");

  await page.getByRole("button", { name: "Signed in as rider@example.test" }).click();
  await page.getByRole("menuitem", { name: "Sign out" }).click();
  await page.waitForURL(/\/auth\/login$/);

  await page.goto("/");
  await expect(page).toHaveURL(/\/auth\/login$/);
});
