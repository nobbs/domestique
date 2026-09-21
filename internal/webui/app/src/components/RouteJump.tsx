/**
 * The ⌘K jump: a route by name, from any page, in a few keystrokes.
 *
 * It lives in the menu bar and owns its own state — what was typed, which row is
 * active — because it belongs to no page. Pages with a search of their own keep
 * ⌘K for it: the catalogue's field narrows the list in place, and the planner's
 * finds a place. There the jump is a button only. An admin on a deployment that
 * plans also finds their drafts here, marked as such, which open in the planner.
 */

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { IconArrowDown, IconArrowUp, IconCornerDownLeft, IconSearch } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { getListPlansQueryOptions } from "../api/generated";
import { routesQuery, webUIConfigQuery } from "../api/queries";
import { routeKey } from "../api/types";
import { formatAscent, formatDistance, formatMovingTime } from "../lib/format";
import { useEffectiveAdmin } from "../lib/identity";
import { matchesText, matchingRoutes, routePath } from "../lib/library";
import { Badge } from "./ui/badge";
import { Dialog, DialogOverlay, DialogPortal } from "./ui/dialog";

/** One row the jump can open: a published route, or one of the admin's drafts. */
interface Entry {
  key: string;
  title: string;
  to: string;
  draft: boolean;
  distanceMetres: number;
  ascentMetres: number;
  movingSeconds?: number;
}

/** Whether the page at this path answers ⌘K with a search of its own. */
export function ownsShortcut(pathname: string): boolean {
  return pathname === "/catalogue" || pathname === "/plan" || pathname.startsWith("/plan/");
}

/** The menu bar's jump button and the panel it opens. */
export function RouteJump() {
  const { pathname, state } = useLocation();
  const navigate = useNavigate();
  const shortcut = !ownsShortcut(pathname);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const field = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);

  const routes = useQuery(routesQuery());
  const library = useMemo(() => routes.data ?? [], [routes.data]);
  const config = useQuery(webUIConfigQuery());
  const planner = useEffectiveAdmin() && config.data?.planning === true;
  // Asked for only while the panel is up, and only by an admin on a deployment that plans.
  const plans = useQuery({ ...getListPlansQueryOptions(), enabled: open && planner });
  const drafts = useMemo(
    () =>
      planner
        ? (plans.data?.data.plans ?? [])
            .filter((plan) => !plan.published)
            .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
        : [],
    [planner, plans.data],
  );
  // Drafts first: an admin's work in progress is the likelier target than the library at large.
  const shown = useMemo<Entry[]>(
    () => [
      ...drafts
        .filter((plan) => matchesText(plan.name, query))
        .map((plan) => ({
          key: `draft/${plan.id}`,
          title: plan.name,
          to: `/plan/${plan.id}`,
          draft: true,
          distanceMetres: plan.distanceMetres,
          ascentMetres: plan.ascentMetres,
        })),
      ...matchingRoutes(library, query).map((route) => ({
        key: routeKey(route),
        title: route.title,
        to: routePath(route),
        draft: false,
        distanceMetres: route.distanceMetres,
        ascentMetres: route.ascentMetres,
        ...(route.movingSeconds === undefined ? {} : { movingSeconds: route.movingSeconds }),
      })),
    ],
    [drafts, library, query],
  );

  useEffect(() => {
    if (!shortcut) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    };
    window.addEventListener("keydown", onKey);

    return () => window.removeEventListener("keydown", onKey);
  }, [shortcut]);

  // Every opening starts over: an empty field on the top row.
  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
    }
  }, [open]);

  const clampedActive = shown.length === 0 ? 0 : Math.min(active, shown.length - 1);
  useEffect(() => {
    list.current
      ?.querySelector(`[data-index="${clampedActive}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [clampedActive]);

  const pick = (index: number) => {
    const target = shown[index];
    if (!target) {
      return;
    }
    setOpen(false);
    // Carried along, so a route reached by jumping still closes to the catalogue it came from.
    navigate(target.to, target.draft ? undefined : { state });
  };
  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive(Math.min(clampedActive + 1, shown.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive(Math.max(clampedActive - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      pick(clampedActive);
    }
  };

  // The catalogue's own field is the search there.
  if (pathname === "/catalogue") {
    return null;
  }

  return (
    <>
      <button
        type="button"
        aria-label="Jump to a route"
        onClick={() => setOpen(true)}
        className="inline-flex h-8 shrink-0 items-center gap-2 rounded-[9px] bg-[var(--muted)] px-2.5 text-[var(--ink-2)] text-sm hover:text-[var(--ink)]"
      >
        <IconSearch size={15} stroke={1.8} aria-hidden="true" />
        <span className="hidden sm:inline">Routes</span>
        {shortcut ? (
          <kbd className="hidden rounded-[6px] bg-[var(--panel)] px-1.5 font-sans text-xs sm:inline">
            ⌘K
          </kbd>
        ) : null}
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogPortal>
          <DialogOverlay />
          <DialogPrimitive.Popup
            aria-label="Jump to a route"
            initialFocus={field}
            className="fixed top-[10vh] left-1/2 z-50 flex h-fit max-h-[70vh] w-[36rem] max-w-[calc(100vw-2rem)] -translate-x-1/2 flex-col overflow-hidden rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] outline-none"
            onKeyDown={onKeyDown}
          >
            <label className="flex items-center gap-3 border-[var(--rule)] border-b px-5 py-4">
              <IconSearch
                size={20}
                stroke={1.8}
                className="text-[var(--ink-2)]"
                aria-hidden="true"
              />
              <input
                ref={field}
                type="search"
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setActive(0);
                }}
                placeholder="Route name or place"
                aria-label="Search the route library"
                aria-activedescendant={
                  shown[clampedActive] ? `jump-option-${shown[clampedActive].key}` : undefined
                }
                className="min-w-0 flex-1 bg-transparent text-lg outline-none placeholder:text-[var(--ink-2)] [&::-webkit-search-cancel-button]:appearance-none"
              />
            </label>
            <ul ref={list} role="listbox" className="flex min-h-0 flex-col overflow-y-auto p-2">
              {shown.length === 0 ? (
                <li className="px-3 py-6 text-center text-[var(--ink-2)] text-sm">
                  Nothing here is called that.
                </li>
              ) : (
                shown.map((entry, index) => {
                  const { key } = entry;
                  const isActive = index === clampedActive;

                  return (
                    <li
                      key={key}
                      id={`jump-option-${key}`}
                      role="option"
                      aria-selected={isActive}
                      data-index={index}
                      onMouseMove={() => setActive(index)}
                      onClick={() => pick(index)}
                      className={`flex cursor-pointer items-center gap-3 rounded-[9px] px-3 py-2 ${
                        isActive ? "bg-[var(--muted)]" : ""
                      }`}
                    >
                      <span className="flex min-w-0 flex-1 items-center gap-2">
                        <span className="truncate font-medium text-sm">{entry.title}</span>
                        {entry.draft ? <Badge variant="secondary">Draft</Badge> : null}
                      </span>
                      <span className="flex gap-3 text-[var(--ink-2)] text-xs tabular-nums">
                        <span className="font-semibold text-[var(--ink)]">
                          {formatDistance(entry.distanceMetres)}
                        </span>
                        <span>{formatAscent(entry.ascentMetres)}</span>
                        {entry.draft ? null : <span>{formatMovingTime(entry.movingSeconds)}</span>}
                      </span>
                    </li>
                  );
                })
              )}
            </ul>
            <div className="flex items-center gap-4 border-[var(--rule)] border-t px-5 py-2.5 text-[var(--ink-2)] text-xs">
              <span>
                {shown.length === library.length + drafts.length
                  ? `${library.length} routes${drafts.length > 0 ? ` · ${drafts.length} drafts` : ""}`
                  : `${shown.length} of ${library.length + drafts.length}`}
              </span>
              <span className="ml-auto flex items-center gap-1">
                <IconArrowUp size={12} />
                <IconArrowDown size={12} /> move
              </span>
              <span className="flex items-center gap-1">
                <IconCornerDownLeft size={12} /> open route
              </span>
              <span>esc close</span>
            </div>
          </DialogPrimitive.Popup>
        </DialogPortal>
      </Dialog>
    </>
  );
}
