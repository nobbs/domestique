/**
 * How sharp a forecast this far ahead can honestly be, as a grid size and as a
 * sentence a reader can act on.
 *
 * Open-Meteo's response carries no model name, so resolution is inferred from
 * lead time the way an operator reading its documentation would: within about
 * two days the answer comes from ICON-D2 at roughly 2 km, beyond that from
 * ICON-EU or the global model at 7–11 km, and past 78 hours from coarser
 * global guidance again.
 *
 * Shared substrate rather than one layout's private detail, for the reason
 * `weather.ts` gives about its own tables: the strip writes the sentence under
 * itself, and anything drawing a forecast on the ground has to know how wide a
 * grid cell is before it can decide how vaguely to draw it. Two copies of these
 * thresholds would eventually disagree about which model a ride is reading.
 */

/** A grid the forecast is really resolved at, and how to say so in one line. */
export interface ForecastResolution {
  /**
   * The forecast model's grid spacing on the ground. The width a reading may
   * be claimed to describe: nothing drawn from it means anything finer.
   */
  metresPerCell: number;
  sentence: string;
}

/** Past this many hours ahead, ICON-D2's 2 km grid is no longer the source. */
const FINE_LEAD_HOURS = 48;

/** And past this, even the 7–11 km regional and global runs have run out. */
const REGIONAL_LEAD_HOURS = 78;

/**
 * The resolution behind a forecast `leadHours` ahead of now.
 *
 * A forecast eleven days out looks exactly as confident as one for tomorrow
 * morning unless something says otherwise, and it is not.
 */
export function forecastResolution(leadHours: number): ForecastResolution {
  if (leadHours <= FINE_LEAD_HOURS) {
    return {
      metresPerCell: 2000,
      sentence: "Within 2 days out, so this uses ICON-D2 — about 2 km resolution.",
    };
  }
  if (leadHours <= REGIONAL_LEAD_HOURS) {
    return {
      metresPerCell: 7000,
      sentence: "More than 2 days out: ICON-EU/global guidance, about 7–11 km resolution.",
    };
  }

  return {
    metresPerCell: 11000,
    sentence: "More than 3 days out: coarser global guidance, past ICON's finer-grained range.",
  };
}

/**
 * The resolution behind a forecast whose first reading lands at `arrivalAt`,
 * counted from now. No arrival is a ride with nothing forecast for it yet,
 * which reads as the sharpest grid rather than the coarsest.
 *
 * Here rather than in each layer that draws the forecast on the ground: the
 * corridor's width comes from this figure, and two copies of the arithmetic
 * would eventually disagree about how wide one ride's corridor is.
 */
export function arrivalResolution(arrivalAt: Date | undefined): ForecastResolution {
  const leadHours = arrivalAt ? Math.max(0, (arrivalAt.getTime() - Date.now()) / 3_600_000) : 0;

  return forecastResolution(leadHours);
}
