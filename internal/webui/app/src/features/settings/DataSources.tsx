/**
 * Every credit this service owes, in the one place a reader can find them all.
 *
 * The tile credits are read out of each configured style document rather than
 * written down here, so they stay correct when an operator points a basemap at
 * a different provider. Every basemap is read, not only the one on screen: a
 * reader may switch to any of them, and this page is not map-contextual.
 *
 * Distinct credits, not one row per basemap. Several entries usually come from
 * one provider declaring one thing — five OpenFreeMap styles credit OpenFreeMap
 * five times — and the obligation is to show the credit, not to map it back to
 * a picker entry.
 *
 * Only the light style of each entry is read. A dark twin must be on its own
 * entry's origin, so it is the same provider crediting itself the same way, and
 * fetching it would double the requests to say so twice.
 */

import { useQueries, useQuery } from "@tanstack/react-query";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { webUIConfigQuery } from "../../api/queries";
import {
  basemapAttributionQuery,
  SURFACE_ATTRIBUTION,
  WEATHER_ATTRIBUTION,
} from "../../lib/attribution";

interface Source {
  label: string;
  credit: string;
}

export function DataSources() {
  const config = useQuery(webUIConfigQuery());
  const basemaps = config.data?.basemaps ?? [];

  const tileCredits = useQueries({
    queries: basemaps.map((basemap) => basemapAttributionQuery(basemap.styleUrl)),
  });

  const mapCredit = [
    ...new Set(tileCredits.map((credit) => credit.data ?? "").filter((credit) => credit !== "")),
  ].join(" · ");

  const sources: Source[] = [
    ...(mapCredit === "" ? [] : [{ label: "Map", credit: mapCredit }]),
    { label: "Surface", credit: SURFACE_ATTRIBUTION },
    { label: "Weather", credit: WEATHER_ATTRIBUTION },
  ];

  return (
    <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
      <CardHeader>
        <CardTitle role="heading" aria-level={2}>
          Data sources
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="mb-4 text-sm text-muted-foreground">
          What this service draws, classifies and forecasts with.
        </p>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          {sources.map((source) => (
            <div key={source.label} className="col-span-2 grid grid-cols-subgrid">
              <dt className="text-muted-foreground">{source.label}</dt>
              <dd>{source.credit}</dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  );
}
