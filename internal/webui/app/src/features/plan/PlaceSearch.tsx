/**
 * The planner's way to a place by name: a pill over the map, or ⌘⇧K, opens a
 * command panel in the atlas's shape. Enter adds the highlighted place; Shift+
 * Enter or a row's box marks it and keeps searching, so several are added at
 * once. Where each lands in the route is the planner's business, not this.
 */

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import {
  IconArrowDown,
  IconArrowUp,
  IconBuildingCommunity,
  IconCornerDownLeft,
  IconHome,
  IconMapPin,
  IconMapPinSearch,
  IconMountain,
  IconSearch,
  IconSquare,
  IconSquareCheckFilled,
  IconTrain,
  IconX,
} from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useMap } from "react-map-gl/maplibre";
import { getSearchPlacesQueryOptions } from "../../api/generated";
import type { PlaceMatch, PlaceSearch as PlaceSearchResult } from "../../api/types";
import { Button } from "../../components/Button";
import { Dialog, DialogOverlay, DialogPortal } from "../../components/ui/dialog";
import { Spinner } from "../../components/ui/spinner";
import { formatDistance } from "../../lib/format";
import { usePrefersReducedMotion } from "../../lib/mediaQuery";
import { haversineMetres } from "../../lib/profile";
import { useSearchPalette } from "../../lib/searchPalette";

/** Fewer characters than this match too much to be worth asking; the service refuses them too. */
const MINIMUM_QUERY = 3;
/** How long typing pauses before the geocoder is asked, in milliseconds. */
const DEBOUNCE_MS = 300;

const KIND_ICON: Record<PlaceMatch["kind"], ReactNode> = {
  address: <IconHome size={16} />,
  settlement: <IconBuildingCommunity size={16} />,
  station: <IconTrain size={16} />,
  peak: <IconMountain size={16} />,
  place: <IconMapPin size={16} />,
};

interface Near {
  latitude: number;
  longitude: number;
}

const keyOf = (place: PlaceMatch) => `${place.name}@${place.latitude},${place.longitude}`;

function useDebounced(value: string): string {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [value]);

  return settled;
}

function PlaceRow({ place, near }: { place: PlaceMatch; near: Near | null }) {
  return (
    <>
      <span
        aria-hidden="true"
        className="grid size-7 shrink-0 place-items-center rounded-md bg-[var(--muted)] text-[var(--ink-2)]"
      >
        {KIND_ICON[place.kind]}
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{place.name}</span>
        {place.context ? (
          <span className="truncate text-[var(--ink-2)] text-xs">{place.context}</span>
        ) : null}
      </span>
      {near ? (
        <span className="shrink-0 text-[var(--ink-2)] text-xs tabular-nums">
          {formatDistance(
            haversineMetres([near.longitude, near.latitude], [place.longitude, place.latitude]),
          )}
        </span>
      ) : null}
    </>
  );
}

export interface PlaceSearchProps {
  /** Places in the order they were chosen. */
  onAdd: (places: PlaceMatch[]) => void;
  disabled?: boolean;
}

/** Rendered as map furniture: it reads the camera to bias the search and to show what it added. */
export function PlaceSearch({ onAdd, disabled = false }: PlaceSearchProps) {
  const { current: map } = useMap();
  const reducedMotion = usePrefersReducedMotion();
  const palette = useSearchPalette();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const [marked, setMarked] = useState<PlaceMatch[]>([]);
  // Taken when the panel opens, so panning behind it does not ask again.
  const [near, setNear] = useState<Near | null>(null);
  const field = useRef<HTMLInputElement>(null);
  const settled = useDebounced(query.trim());
  const asking = settled.length >= MINIMUM_QUERY;
  const search = useQuery({
    ...getSearchPlacesQueryOptions(
      near
        ? {
            query: settled,
            latitude: Number(near.latitude.toFixed(3)),
            longitude: Number(near.longitude.toFixed(3)),
          }
        : { query: settled },
      { query: { select: (response) => (response.data as PlaceSearchResult).places } },
    ),
    enabled: open && asking,
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
  // Answers count only once they are for the text in the field: Enter must never
  // add a place from the search before, while this one is still pausing or asking.
  const current = asking && settled === query.trim();
  const hits = current ? (search.data ?? []) : [];

  const show = (next: boolean) => {
    if (next) {
      const centre = map?.getCenter();
      setNear(centre ? { latitude: centre.lat, longitude: centre.lng } : null);
      requestAnimationFrame(() => field.current?.focus());
    } else {
      // A closed panel forgets its search and its marks: nothing was added.
      setQuery("");
      setMarked([]);
    }
    setOpen(next);
  };

  useEffect(() => {
    // Each chord closes the other's panel, so the two never stack.
    const onKey = (event: KeyboardEvent) => {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== "k") {
        return;
      }
      if (!event.shiftKey) {
        if (open) {
          show(false);
        }
      } else if (!disabled) {
        event.preventDefault();
        palette.setOpen(false);
        show(!open);
      }
    };
    window.addEventListener("keydown", onKey);

    return () => window.removeEventListener("keydown", onKey);
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: only new answers restart the list
  useEffect(() => {
    setActive(0);
  }, [search.data]);

  const isMarked = (place: PlaceMatch) => marked.some((entry) => keyOf(entry) === keyOf(place));
  const toggle = (place: PlaceMatch) => {
    setMarked((current) =>
      current.some((entry) => keyOf(entry) === keyOf(place))
        ? current.filter((entry) => keyOf(entry) !== keyOf(place))
        : [...current, place],
    );
    setQuery("");
    field.current?.focus();
  };
  const reveal = (places: PlaceMatch[]) => {
    const view = map?.getBounds();
    if (
      !map ||
      !view ||
      places.every((place) => view.contains([place.longitude, place.latitude]))
    ) {
      return;
    }
    for (const place of places) {
      view.extend([place.longitude, place.latitude]);
    }
    map.fitBounds(view, { padding: 56, duration: reducedMotion ? 0 : 600 });
  };
  const commit = (extra?: PlaceMatch) => {
    const places = extra && !isMarked(extra) ? [...marked, extra] : marked;
    if (places.length === 0) {
      return;
    }
    onAdd(places);
    reveal(places);
    show(false);
  };
  const highlighted = hits[Math.min(active, hits.length - 1)];
  const enterAdds = marked.length + (highlighted && !isMarked(highlighted) ? 1 : 0);

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.target !== field.current) {
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((index) => Math.min(index + 1, hits.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((index) => Math.max(index - 1, 0));
    } else if (event.key === "Enter" && event.shiftKey) {
      event.preventDefault();
      if (highlighted) {
        toggle(highlighted);
      }
    } else if (event.key === "Enter") {
      event.preventDefault();
      commit(highlighted);
    } else if (event.key === "Backspace" && query === "" && marked.length > 0) {
      setMarked((current) => current.slice(0, -1));
    }
  };

  const typed = query.trim().length;
  const hint =
    typed === 0
      ? marked.length > 0
        ? "Search for the next place, or press Enter to add these."
        : "Search for a place, an address, a station or a peak."
      : typed < MINIMUM_QUERY
        ? "Keep typing…"
        : !current || search.isPending
          ? "Searching…"
          : search.isError
            ? "Place search is unavailable just now."
            : hits.length === 0
              ? "No place by that name."
              : null;

  return (
    <>
      <Button
        variant="panel"
        icon={<IconMapPinSearch stroke={1.8} />}
        className="w-56 justify-start"
        disabled={disabled}
        onClick={() => show(true)}
      >
        <span className="flex-1 text-left">Search places</span>
        <kbd className="rounded-[7px] bg-[var(--muted)] px-1.5 py-0.5 font-sans text-xs">⌘⇧K</kbd>
      </Button>
      <Dialog open={open} onOpenChange={show}>
        <DialogPortal>
          <DialogOverlay />
          <DialogPrimitive.Popup
            aria-label="Search for a place"
            className="-translate-x-1/2 fixed top-[10vh] left-1/2 z-50 flex h-fit max-h-[70vh] w-[36rem] max-w-[calc(100vw-2rem)] flex-col overflow-hidden rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] outline-none"
            onKeyDown={onKeyDown}
          >
            <label className="flex flex-wrap items-center gap-2 border-[var(--rule)] border-b px-5 py-4">
              <IconSearch
                size={20}
                stroke={1.8}
                className="text-[var(--ink-2)]"
                aria-hidden="true"
              />
              {marked.map((place) => (
                <span
                  key={keyOf(place)}
                  className="flex h-7 items-center gap-1 rounded-[8px] bg-[color-mix(in_oklab,var(--accent)_16%,transparent)] pr-1 pl-2 text-sm"
                >
                  {place.name}
                  <button
                    type="button"
                    aria-label={`Unmark ${place.name}`}
                    className="grid size-5 place-items-center rounded text-[var(--ink-2)] hover:text-[var(--ink)]"
                    onClick={() => toggle(place)}
                  >
                    <IconX size={12} />
                  </button>
                </span>
              ))}
              <input
                ref={field}
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={marked.length > 0 ? "And…" : "Place or address"}
                aria-label="Search for a place"
                maxLength={200}
                className="min-w-32 flex-1 bg-transparent text-lg outline-none placeholder:text-[var(--ink-2)] [&::-webkit-search-cancel-button]:appearance-none"
              />
              {search.isFetching ? <Spinner /> : null}
            </label>
            <ul
              role="listbox"
              aria-label="Places"
              className="flex min-h-0 flex-col overflow-y-auto p-2"
            >
              {hint ? (
                <li className="px-3 py-6 text-center text-[var(--ink-2)] text-sm">{hint}</li>
              ) : (
                hits.map((place, index) => {
                  const isActive = place === highlighted;
                  const checked = isMarked(place);

                  return (
                    <li
                      key={keyOf(place)}
                      role="option"
                      aria-selected={isActive}
                      onMouseMove={() => setActive(index)}
                      onClick={() => commit(place)}
                      className={`flex cursor-pointer items-center gap-3 rounded-[9px] px-2 py-2 text-sm ${isActive ? "bg-[var(--muted)]" : ""}`}
                    >
                      <button
                        type="button"
                        aria-label={checked ? `Unmark ${place.name}` : `Mark ${place.name}`}
                        aria-pressed={checked}
                        className={`grid size-7 shrink-0 place-items-center rounded-md ${checked ? "text-[var(--accent)]" : "text-[var(--ink-2)] hover:text-[var(--ink)]"}`}
                        onClick={(event) => {
                          event.stopPropagation();
                          toggle(place);
                        }}
                      >
                        {checked ? <IconSquareCheckFilled size={18} /> : <IconSquare size={18} />}
                      </button>
                      <PlaceRow place={place} near={near} />
                      {isActive ? (
                        <IconCornerDownLeft
                          size={14}
                          className="text-[var(--ink-2)]"
                          aria-hidden="true"
                        />
                      ) : (
                        <span className="w-3.5" />
                      )}
                    </li>
                  );
                })
              )}
            </ul>
            <div className="flex items-center gap-4 border-[var(--rule)] border-t px-5 py-2.5 text-[var(--ink-2)] text-xs">
              <span className="flex items-center gap-1">
                <IconArrowUp size={12} />
                <IconArrowDown size={12} /> move
              </span>
              <span className="flex items-center gap-1">
                ⇧<IconCornerDownLeft size={12} /> mark
              </span>
              <span className="flex items-center gap-1">
                <IconCornerDownLeft size={12} />
                {enterAdds > 1 ? `add ${enterAdds}` : "add"}
              </span>
              <span>esc close</span>
              {marked.length > 0 ? (
                <Button className="ml-auto" onClick={() => commit()}>
                  Add {marked.length} {marked.length === 1 ? "place" : "places"}
                </Button>
              ) : null}
            </div>
          </DialogPrimitive.Popup>
        </DialogPortal>
      </Dialog>
    </>
  );
}
