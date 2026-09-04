/**
 * The two things a forecast has to be drawn with, rather than written out:
 * a glyph for the condition, and a band for the temperature.
 *
 * Both are shared substrate rather than one layout's private detail. Every
 * alternative that shows conditions at all wants the same mapping, and three
 * hand-rolled WMO tables would make a comparison between layouts turn into a
 * comparison between mappings.
 *
 * The labels those codes read as still live in `ForecastStrip`, which is the
 * only thing that needs them; if a layout that draws icons wins, the table
 * and this module belong together.
 */

import {
  IconCloud,
  IconCloudFilled,
  IconCloudFog,
  IconCloudRain,
  IconCloudSnow,
  IconCloudStorm,
  IconDroplets,
  type IconProps,
  IconSnowflake,
  IconSun,
  IconSunHigh,
} from "@tabler/icons-react";
import type { ComponentType } from "react";

/**
 * Open-Meteo's WMO codes, as glyphs.
 *
 * Coarser than the label table on purpose: "moderate" and "dense" drizzle are
 * worth different words but not different pictures, and a 16px glyph that
 * tries to draw the difference draws neither. Intensity is the temperature
 * band's and the precipitation figure's job.
 */
const WEATHER_CODE_ICONS: Record<number, ComponentType<IconProps>> = {
  0: IconSun,
  1: IconSunHigh,
  2: IconCloud,
  3: IconCloudFilled,
  45: IconCloudFog,
  48: IconCloudFog,
  51: IconDroplets,
  53: IconDroplets,
  55: IconDroplets,
  56: IconSnowflake,
  57: IconSnowflake,
  61: IconCloudRain,
  63: IconCloudRain,
  65: IconCloudRain,
  66: IconSnowflake,
  67: IconSnowflake,
  71: IconCloudSnow,
  73: IconCloudSnow,
  75: IconCloudSnow,
  77: IconCloudSnow,
  80: IconCloudRain,
  81: IconCloudRain,
  82: IconCloudRain,
  85: IconCloudSnow,
  86: IconCloudSnow,
  95: IconCloudStorm,
  96: IconCloudStorm,
  99: IconCloudStorm,
};

/**
 * The glyph for a weather code, or `IconCloud` for one this table has never
 * seen. A code the provider adds later draws as unremarkable weather rather
 * than as nothing at all, which is the honest guess: the strip's own label
 * still reports it by number.
 */
export function weatherIcon(code: number): ComponentType<IconProps> {
  return WEATHER_CODE_ICONS[code] ?? IconCloud;
}

/**
 * Where each band starts, in Celsius, from the top down.
 *
 * Cut for a rider rather than for a thermometer: the boundaries are the
 * points at which what goes on over the jersey changes — long sleeves under
 * 12, a gilet under 5, and above 27 the ride itself becomes the problem.
 */
export const TEMPERATURE_FLOORS = [27, 20, 12, 5] as const;

/** Which of the five `--temp-*` bands a reading falls in, 0 coldest. */
export function temperatureBand(celsius: number): 0 | 1 | 2 | 3 | 4 {
  const above = TEMPERATURE_FLOORS.findIndex((floor) => celsius >= floor);

  return (above === -1 ? 0 : 4 - above) as 0 | 1 | 2 | 3 | 4;
}

/** The custom property a reading is painted from. */
export function temperatureColour(celsius: number): string {
  return `var(--temp-${temperatureBand(celsius)})`;
}
