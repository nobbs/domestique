/**
 * The panel every alternative is shown in: a pill at rest, a card when asked.
 *
 * The elevation chart and the forecast strip are gone from here — they are
 * instruments for studying one route, and this panel exists to answer whether
 * a route is the one to ride. What is left is small enough that covering the
 * map with it permanently is not worth it, so the resting state is a line: the
 * route's name and the two figures a ride is decided on.
 *
 * The mechanism is the search panel's own. `SearchPanel` already rests as a
 * bare icon over the map and marks itself `data-compact-workspace`, on which
 * the shell drops its background, padding, shadow and ring — so a panel that
 * brings its own chrome gets a floating pill for free, and the camera keeps
 * framing routes around whatever size it currently is.
 *
 * Expanding does not open a second thing. The pill stays as the card's own
 * header and the card unfolds beneath it, so there is one object on the map
 * throughout and the control that opened it is the control that closes it.
 */

import { IconChevronsRight, IconDots, IconX } from "@tabler/icons-react";
import type { ComponentType } from "react";
import { SourceRouteLink } from "../../../components/SourceRouteLink";
import { Popover, PopoverContent, PopoverTrigger } from "../../../components/ui/popover";
import { formatAscent, formatDistance } from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import { ReprocessButton } from "../ReprocessButton";
import type { CardProps } from "./shared";

export interface SpikePanelProps extends Omit<CardProps, "highlight" | "onHighlightChange"> {
  /** The alternative on show. Swapping this is the whole experiment. */
  Card: ComponentType<CardProps>;
  /**
   * Whether the panel rests as a pill, held by the caller.
   *
   * Sticky across routes in the real page, for the reason `RouteProfile`
   * already gives about its chart: a reader who put the card away did so to
   * see more map, not to see more of one route's map.
   */
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
  onClose: () => void;
  sourceBaseUrls: Record<string, string>;
  /** How many routes the search behind this panel goes back to. */
  libraryCount: number;
}

export function SpikePanel({
  Card,
  collapsed,
  onCollapsedChange,
  onClose,
  sourceBaseUrls,
  libraryCount,
  ...card
}: SpikePanelProps) {
  const { route, unitSystem, onHighlightChange } = card;

  return (
    // The shell strips its own card off whatever carries this, which is what
    // lets the pill be a pill rather than a pill inside a panel.
    <div data-compact-workspace="" className="w-fit max-w-full">
      <section
        aria-label={route.title}
        className="overflow-hidden rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] ring-1 ring-black/5"
      >
        <div className="flex items-center gap-1 p-1.5">
          <button
            type="button"
            aria-expanded={!collapsed}
            onClick={() => {
              const next = !collapsed;
              onCollapsedChange(next);
              // Collapsing takes the chips away with it, and the pressed chip
              // is the only way to give the whole route back. Rather than
              // leave the map lit with no visible cause, putting the card
              // away puts the question away too.
              if (next) {
                onHighlightChange(null);
              }
            }}
            className="flex min-w-0 items-center gap-2 rounded-lg px-2 py-1 text-left hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
          >
            <IconChevronsRight
              size={16}
              stroke={2}
              aria-hidden="true"
              className={collapsed ? "transition-transform" : "rotate-90 transition-transform"}
            />
            <span className="max-w-[15rem] truncate font-semibold">{route.title}</span>
            {/*
             * The two figures a ride is decided on, on the line that is visible
             * far more often than the card is. A pill that only named the route
             * would make every reading of it cost a press.
             */}
            <span className="shrink-0 text-sm text-[var(--ink-2)] tabular-nums">
              {formatDistance(route.distanceMetres, unitSystem)} ·{" "}
              {formatAscent(route.ascentMetres, unitSystem)}
            </span>
          </button>
          <Popover>
            <PopoverTrigger
              aria-label="More about this route"
              className="rounded-lg p-1.5 text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
            >
              <IconDots size={16} stroke={2} aria-hidden="true" />
            </PopoverTrigger>
            <PopoverContent align="end" className="grid w-auto gap-2 p-2">
              {/*
               * The two quiet actions that used to hold a bordered row of their
               * own at the foot of the card. Both are rare — one leaves for the
               * provider, the other asks the service to work the route out
               * again — and a row spent on them is a row not spent on the route.
               */}
              <SourceRouteLink
                provider={route.provider}
                baseUrl={sourceBaseUrls[route.provider]}
                sourceRouteId={route.sourceRouteId}
              />
              <ReprocessButton
                provider={route.provider}
                sourceRouteId={route.sourceRouteId}
                stageOrder={route.stageOrder}
              />
              <p className="max-w-[16rem] border-t border-[var(--rule)] pt-2 text-xs text-[var(--ink-2)]">
                The elevation profile and the forecast open in the wide panel, which does not exist
                yet — its control belongs in this menu.
              </p>
            </PopoverContent>
          </Popover>
          <button
            type="button"
            onClick={onClose}
            // The count is what makes leaving a described action rather than an
            // undo. The card no longer has a row to write it on, so the button
            // says it where a name is read instead of where one is drawn.
            aria-label={`Close the route and go back to ${libraryCount} routes`}
            title={`Back to ${libraryCount} routes`}
            className="rounded-lg p-1.5 text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
          >
            <IconX size={16} stroke={2} aria-hidden="true" />
          </button>
        </div>
        {collapsed ? null : (
          <div className="w-[30rem] max-w-full border-t border-[var(--rule)] px-4 pt-3 pb-4">
            <Card {...card} />
          </div>
        )}
      </section>
    </div>
  );
}
