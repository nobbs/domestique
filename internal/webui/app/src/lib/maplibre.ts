/**
 * One-time MapLibre configuration, imported for its side effect by every module
 * that creates a map.
 *
 * MapLibre builds its worker filename at runtime, which the bundler's static
 * analysis cannot see, so the worker chunk is never emitted and tile parsing
 * silently never starts: the map fetches its style and sprites, renders its
 * background, and then requests no tiles or glyphs at all.
 *
 * `?worker&url` makes the worker an explicit build input and yields a
 * same-origin URL the Content-Security-Policy allows. It must be `?worker&url`
 * rather than plain `?url`: the latter copies the file verbatim, leaving its
 * `./maplibre-gl-shared.mjs` import unresolved.
 *
 * This lives here rather than beside one map component because every map in the
 * application shares the worker pool. A component that creates a map without
 * this import gets no tiles.
 */

import { setWorkerUrl } from "maplibre-gl";
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url";

setWorkerUrl(workerUrl);
