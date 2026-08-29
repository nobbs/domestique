/**
 * Every credit this service owes, in one place. Distinct credits rather than one
 * row per basemap: several entries usually come from one provider.
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

  // Every configured basemap, not only the one on screen: a reader may switch
  // to any of them, and this page does not know which is loaded.
  const tileCredits = useQueries({
    queries: basemaps.map((basemap) =>
      basemapAttributionQuery(basemap.styleUrl, basemap.styleUrlDark),
    ),
  });

  const mapCredit = [...new Set(tileCredits.flatMap((credit) => credit.data ?? []))].join(" · ");

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
