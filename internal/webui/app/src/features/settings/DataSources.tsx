/**
 * Every credit this service owes, in one place, each a link to its provider
 * where the provider gives one. Several basemaps usually share a provider.
 */

import {
  type Icon,
  IconBook,
  IconCloudRain,
  IconMap,
  IconRoad,
  IconRoute,
} from "@tabler/icons-react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Panel } from "@/components/PanelHeading";
import { webUIConfigQuery } from "../../api/queries";
import {
  basemapAttributionQuery,
  type Credit,
  ROUTING_CREDITS,
  SURFACE_CREDITS,
  uniqueCredits,
  WEATHER_CREDITS,
} from "../../lib/attribution";

interface Source {
  label: string;
  use: string;
  icon: Icon;
  credits: Credit[];
}

const CHIP =
  "rounded-full bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] px-2.5 py-1 text-xs";

export function DataSources() {
  const config = useQuery(webUIConfigQuery());
  const basemaps = config.data?.basemaps ?? [];

  // Every configured basemap, not only the one on screen: a reader may switch
  // to any of them, and this page does not know which is loaded.
  const tileCredits = useQueries({
    queries: basemaps.map((basemap) =>
      basemapAttributionQuery(basemap.styleUrl, basemap.styleUrlDark),
    ),
  });
  const mapCredits = uniqueCredits(tileCredits.flatMap((credit) => credit.data ?? []));

  const sources: Source[] = [
    ...(mapCredits.length === 0
      ? []
      : [{ label: "Map", use: "Draws the basemap", icon: IconMap, credits: mapCredits }]),
    {
      label: "Surface",
      use: "Classifies what each stretch is paved with",
      icon: IconRoad,
      credits: SURFACE_CREDITS,
    },
    {
      label: "Weather",
      use: "Forecasts wind and rain along a ride",
      icon: IconCloudRain,
      credits: WEATHER_CREDITS,
    },
    ...(config.data?.planning
      ? [
          {
            label: "Routing",
            use: "Snaps planned routes to the road",
            icon: IconRoute,
            credits: ROUTING_CREDITS,
          },
        ]
      : []),
  ];

  return (
    <Panel
      icon={<IconBook size={18} stroke={1.8} />}
      title="Data sources"
      subtitle="what this service draws, classifies and forecasts with"
    >
      <dl className="flex flex-col divide-y divide-[var(--rule)]">
        {sources.map((source) => (
          <div
            key={source.label}
            className="grid gap-3 py-3.5 first:pt-0 last:pb-0 sm:grid-cols-[14rem_1fr] sm:items-center"
          >
            <dt className="flex items-center gap-3">
              <source.icon
                size={20}
                stroke={1.6}
                className="shrink-0 text-[var(--ink-2)]"
                aria-hidden="true"
              />
              <span className="flex flex-col">
                <span className="font-semibold text-sm">{source.label}</span>
                <span className="text-[var(--ink-2)] text-xs">{source.use}</span>
              </span>
            </dt>
            <dd className="flex flex-wrap gap-1.5">
              {source.credits.map((credit) =>
                credit.href ? (
                  <a
                    key={`${credit.text}${credit.href}`}
                    href={credit.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={`${CHIP} hover:bg-[color-mix(in_oklab,var(--ink-2)_16%,transparent)] focus-visible:outline-2 focus-visible:outline-[var(--accent)] focus-visible:outline-offset-2`}
                  >
                    {credit.text}
                  </a>
                ) : (
                  <span key={credit.text} className={CHIP}>
                    {credit.text}
                  </span>
                ),
              )}
            </dd>
          </div>
        ))}
      </dl>
    </Panel>
  );
}
