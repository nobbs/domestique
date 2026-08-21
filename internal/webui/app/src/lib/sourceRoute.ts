/**
 * The way back from a stored stage to the route it was made from.
 *
 * Domestique shows a stage read-only and writes nothing back, so the route in
 * the provider's own web application is where an operator goes to change
 * anything. Until now getting there meant remembering the route's name and
 * finding it by hand, even though the stage has carried the provider's own
 * immutable route identifier all along.
 *
 * The link is built from the identifier and the configured base URL, never
 * fetched: asking the provider where its own route lives would be a network
 * round trip to learn something already stored, and it would make a read of
 * local state depend on an upstream being reachable.
 *
 * The path is the provider's canonical one and carries no locale segment — the
 * provider applies the operator's own. The base URL comes from configuration, so
 * a differently configured deployment stays correct with no change here.
 *
 * The route it leads to is private to the operator's own provider account. It is
 * a way back to their own library, not a URL to share: signed out, or signed in
 * as somebody else, it resolves to nothing they can read.
 */

/** The provider's canonical web path for one of an account's own routes. */
const SOURCE_ROUTE_PATH = "user-routes";

export interface SourceRoute {
  /** Where the route lives, absolute, for an anchor to point at. */
  href: string;
  /**
   * The provider's host, port included when the base URL names one, so the
   * affordance can say where it leads rather than making a reader hover a link
   * to find out. It is what a reader would read off the address bar after
   * following it, which is the point of saying it beforehand.
   */
  host: string;
  /**
   * The same place as a name rather than as an address: no port, and no `www.`
   * in front. A control is wide enough for the part that identifies a provider
   * to a reader, which is the part that survives dropping both.
   *
   * It stays a part of what `host` says, so an affordance that shows this and
   * announces that is not calling itself two different things.
   */
  name: string;
}

/**
 * The source route link for one stage, or null when there is none to give.
 *
 * Null rather than a best effort, in every case where the answer would be a
 * guess: an unconfigured base URL, one that is not an absolute HTTPS URL, or an
 * identifier that is not a positive whole number. A link that goes nowhere is
 * worse than no link, because a reader cannot tell which of the two they have
 * until they follow it.
 *
 * A base URL that ends in a slash, or that carries a path of its own, is
 * honoured as written: the route path is appended to it rather than replacing
 * it, so a provider hosted under a prefix keeps that prefix.
 */
export function sourceRoute(baseUrl: string | undefined, routeId: number): SourceRoute | null {
  if (!baseUrl || !Number.isInteger(routeId) || routeId <= 0) {
    return null;
  }
  let base: URL;
  try {
    base = new URL(baseUrl);
  } catch {
    return null;
  }
  // HTTPS only, and the identifier is the whole of what travels: no query and no
  // fragment, so following the link tells the provider nothing about the reader
  // beyond which of their own routes they asked for.
  if (base.protocol !== "https:") {
    return null;
  }
  const url = new URL(base.origin);
  url.pathname = `${base.pathname.replace(/\/+$/, "")}/${SOURCE_ROUTE_PATH}/${routeId}`;

  return {
    href: url.toString(),
    host: base.host,
    name: base.hostname.replace(/^www\./, ""),
  };
}
