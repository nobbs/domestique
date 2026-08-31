/** UI-domain aliases over the OpenAPI-generated models. */
import {
  type AlertSetting,
  type BrowserBasemap,
  type Build,
  type WebUIConfig as GeneratedWebUIConfig,
  type GeoJSONFeature,
  NotificationSettingsSuccessPolicy,
  type Route,
  type RouteValidation,
  type Settings,
  type SourceSettings,
  SourceSettingsProvider,
  type Status,
  type SurfaceRange,
  SurfaceRangeKind,
  type SyncActive,
  type SyncPhaseRun,
  type SyncRun,
  type SyncRunPage,
  type SyncSchedule,
  type SyncStatus,
  type TargetRoutes,
  type TargetRun,
  type TargetStatus,
  type TargetStatusConvergence,
  type WahooRateLimit,
  type WeatherForecast,
  type WeatherPoint,
} from "./generated";

export type {
  AlertSetting,
  BrowserBasemap,
  Build as BuildInfo,
  GeoJSONFeature,
  Route,
  RouteValidation,
  Settings,
  SourceSettings,
  Status,
  SurfaceRange,
  SyncActive,
  SyncPhaseRun,
  SyncRun,
  SyncRunPage,
  SyncSchedule,
  SyncStatus,
  TargetRoutes,
  TargetRun,
  TargetStatus,
  WahooRateLimit,
  WeatherForecast,
  WeatherPoint,
};

export type BoundingBox = [number, number, number, number];
export type Position = [number, number] | [number, number, number];

export const SURFACE_KINDS = Object.values(SurfaceRangeKind);
export type SurfaceKind = (typeof SURFACE_KINDS)[number];

export const TARGET_CONVERGENCES = ["current", "lagging", "failed", "unauthorized"] as const;
export type TargetConvergence = TargetStatusConvergence;

export const SYNC_PHASES = ["source", "targets"] as const;
export type SyncPhase = (typeof SYNC_PHASES)[number];

export const SUCCESS_POLICIES = Object.values(NotificationSettingsSuccessPolicy);
export type SuccessPolicy = (typeof SUCCESS_POLICIES)[number];

export const SOURCE_PROVIDERS = Object.values(SourceSettingsProvider);
export type SourceProvider = (typeof SOURCE_PROVIDERS)[number];

/** Defaults optional transport fields to the values the UI renders. */
export type Basemap = Omit<BrowserBasemap, "darkCartography"> & { darkCartography: boolean };
export type WebUIConfig = Omit<GeneratedWebUIConfig, "basemaps" | "sourceBaseUrls"> & {
  basemaps: Basemap[];
  sourceBaseUrls: Record<string, string>;
};

/** The GeoJSON projection the map and profile consume. */
export interface RouteGeometry {
  bbox: BoundingBox;
  coordinates: Position[];
  surface?: RouteSurface | undefined;
  cumulativeSeconds?: number[] | undefined;
}

export interface RouteSurface {
  ranges: SurfaceRange[];
  matchedMetres: number;
}

export function routeGeometry(feature: GeoJSONFeature | RouteGeometry): RouteGeometry {
  if ("coordinates" in feature) {
    return feature;
  }

  return {
    bbox: feature.bbox as BoundingBox,
    coordinates: feature.geometry.coordinates as Position[],
    surface: feature.properties.surface,
    cumulativeSeconds: feature.properties.cumulativeSeconds,
  };
}

export function webUIConfig(config: GeneratedWebUIConfig): WebUIConfig {
  const sourceBaseUrls = config.sourceBaseUrls;

  return {
    ...config,
    basemaps: config.basemaps.map((basemap) => ({
      ...basemap,
      darkCartography: basemap.darkCartography ?? false,
    })),
    sourceBaseUrls: {
      ...(sourceBaseUrls?.veloplanner === undefined
        ? {}
        : { veloplanner: sourceBaseUrls.veloplanner }),
      ...(sourceBaseUrls?.komoot === undefined ? {} : { komoot: sourceBaseUrls.komoot }),
    },
  };
}

/** A route's stable identity, used for routing and list keys. */
export function routeKey(route: Pick<Route, "provider" | "sourceRouteId" | "stageOrder">): string {
  return `${route.provider}/${route.sourceRouteId}/${route.stageOrder}`;
}
