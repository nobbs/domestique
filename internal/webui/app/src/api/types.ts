/** UI-domain aliases over the OpenAPI-generated models. */
import {
  type Activity,
  type ActivityList,
  type ActivityTrackPropertiesState,
  type AlertSetting,
  type BrowserBasemap,
  type Build,
  type ActivityTrack as GeneratedActivityTrack,
  type WebUIConfig as GeneratedWebUIConfig,
  type GeoJSONFeature,
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
  type SyncStatus,
  type TargetRoutes,
  type TargetRun,
  type TargetStatus,
  type TargetStatusConvergence,
  type Task,
  type TaskList,
  type TaskRun,
  type TaskRunPage,
  type WahooRateLimit,
  type WeatherForecast,
  type WeatherPoint,
} from "./generated";

export type {
  Activity,
  ActivityList,
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
  SyncStatus,
  TargetRoutes,
  TargetRun,
  TargetStatus,
  Task,
  TaskList,
  TaskRun,
  TaskRunPage,
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

/**
 * One ride's recorded track, in the shape the map and profile already read.
 * A ride with no line to draw arrives with none, and `state` says why.
 */
export interface ActivityTrack {
  bbox?: BoundingBox | undefined;
  coordinates: Position[];
  state: ActivityTrackState;
}

export type ActivityTrackState = ActivityTrackPropertiesState;

/**
 * Folds the altitudes back into the positions they belong to, which is where
 * `buildActivityProfile` reads elevation from. A track already in that shape
 * passes through, exactly as `routeGeometry` above lets a caller hand over
 * either. A sample with no altitude — absent or `null` — travels as a two-wide
 * position, the same shape a track missing altitude everywhere already used.
 */
export function activityTrack(feature: GeneratedActivityTrack | ActivityTrack): ActivityTrack {
  if ("coordinates" in feature) {
    return feature;
  }
  const altitudes = feature.properties.altitudeMetres;
  const geometry = feature.geometry;
  if (geometry === null) {
    return { coordinates: [], state: feature.properties.state };
  }

  return {
    bbox: feature.bbox as BoundingBox,
    state: feature.properties.state,
    coordinates: geometry.coordinates.map(([longitude = 0, latitude = 0], index) => {
      const altitude = altitudes?.[index];

      return altitude === undefined || altitude === null
        ? ([longitude, latitude] as Position)
        : ([longitude, latitude, altitude] as Position);
    }),
  };
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
