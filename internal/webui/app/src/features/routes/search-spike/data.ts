/**
 * A synthetic library for the atlas search spikes: loops and out-and-backs of
 * varied length and climbing, each with a shape, elevation and surface ranges.
 * Generated from a fixed seed so every story shows the same routes.
 */

import type { Position, Route, RouteGeometry, SurfaceRange } from "../../../api/types";

function lcg(seed: number): () => number {
  let state = seed;
  return () => {
    state = (state * 1_664_525 + 1_013_904_223) % 4_294_967_296;
    return state / 4_294_967_296;
  };
}

const NAMES = [
  "Rheinufer loop",
  "Odenwald hills",
  "Pfälzerwald gravel",
  "Bergstraße classic",
  "Kraichgau rollers",
  "Neckar valley",
  "Königstuhl climb",
  "Hardt forest tracks",
  "Weinstraße tour",
  "Melibokus repeats",
  "Altrhein gravel",
  "Katzenbuckel epic",
  "Lampertheim flats",
  "Felsberg out-and-back",
  "Tromm ridge",
  "Speyer cathedral run",
];

export interface SpikeRoute {
  route: Route;
  geometry: RouteGeometry;
}

function build(): SpikeRoute[] {
  const random = lcg(11);
  const names = [0, 1, 2, 3].flatMap((round) =>
    NAMES.map((name) => (round === 0 ? name : `${name} ${["north", "south", "west"][round - 1]}`)),
  );
  return names.map((title, index) => {
    const km = 25 + Math.round(random() * 130);
    const hilly = random();
    const ascent = Math.round(km * (3 + hilly * 18));
    const points = 160;
    const radius = km / (2 * Math.PI) / 111;
    const outAndBack = index % 4 === 3;
    const dx = ((index * 37) % 11) / 10 - 0.5;
    const dy = ((index * 53) % 9) / 12 - 0.35;
    const wobble = 0.1 + random() * 0.2;
    const lobes = 3 + Math.floor(random() * 4);
    const coordinates: Position[] = Array.from({ length: points }, (_, step) => {
      const t = step / (points - 1);
      const theta = t * Math.PI * 2;
      const r = radius * (1 + wobble * Math.sin(lobes * theta));
      const lon = outAndBack
        ? 8.5 + dx + radius * 2 * Math.sin(theta / 2)
        : 8.5 + dx + r * Math.cos(theta);
      const lat = outAndBack
        ? 49.4 + dy + radius * 0.3 * Math.sin(theta)
        : 49.4 + dy + 0.7 * r * Math.sin(theta);
      const ele =
        110 +
        (ascent / 3) *
          (1 - Math.cos(theta * (1 + Math.round(hilly * 4)))) *
          (0.6 + 0.4 * Math.sin(theta * 7));
      return [lon, lat, Math.max(90, ele)];
    });
    const kinds = ["asphalt", "compacted", "gravel", "ground", "paving"] as const;
    const surface: SurfaceRange[] = [];
    let start = 0;
    while (start < points - 1) {
      const end = Math.min(points - 1, start + 10 + Math.floor(random() * 40));
      const gravelly = title.includes("gravel") || title.includes("tracks");
      const kind = gravelly
        ? (kinds[1 + Math.floor(random() * 3)] ?? "gravel")
        : random() > 0.8
          ? (kinds[Math.floor(random() * 5)] ?? "asphalt")
          : "asphalt";
      surface.push({ kind, startIndex: start, endIndex: end });
      start = end;
    }
    const lons = coordinates.map((c) => c[0]);
    const lats = coordinates.map((c) => c[1]);
    const route: Route = {
      provider: "veloplanner",
      sourceRouteId: 100 + index,
      stageOrder: 1,
      title,
      sourceRouteName: title,
      routeName: title,
      sourceRevision: "2026-09-01",
      contentHash: `spike-${index}`,
      distanceMetres: km * 1000,
      ascentMetres: ascent,
      descentMetres: ascent,
      maxGradientPercent: Math.round((4 + hilly * 12) * 10) / 10,
      pointCount: points,
      movingSeconds: Math.round((km / (27 - hilly * 8)) * 3600),
    };
    return {
      route,
      geometry: {
        bbox: [Math.min(...lons), Math.min(...lats), Math.max(...lons), Math.max(...lats)],
        coordinates,
        surface: { matchedMetres: km * 1000, ranges: surface },
      } as RouteGeometry,
    };
  });
}

export const LIBRARY: SpikeRoute[] = build();
