import { describe, expect, it } from "vitest";
import {
  formatAscent,
  formatCadence,
  formatCount,
  formatDistance,
  formatElevation,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
  formatPrecipitation,
  formatReadTime,
  formatTemperature,
  formatTimestamp,
  formatWindSpeed,
} from "./format";

describe("formatDistance", () => {
  it.each([
    [0, "—"],
    [-5, "—"],
    [Number.NaN, "—"],
    [420, "420 m"],
    [1500, "1.5 km"],
    [42_500, "42.5 km"],
    [180_400, "180 km"],
  ])("formats %p as %p in metric", (metres, expected) => {
    expect(formatDistance(metres, "metric")).toBe(expected);
  });

  it.each([
    [0, "—"],
    [-5, "—"],
    [Number.NaN, "—"],
    [420, "1378 ft"],
    [1609.344, "1.0 mi"],
    [42_500, "26.4 mi"],
    [290_000, "180 mi"],
  ])("formats %p as %p in imperial", (metres, expected) => {
    expect(formatDistance(metres, "imperial")).toBe(expected);
  });

  // A metre count whose feet fall just short of 5280 but round up to it must
  // not print "5280 ft" — the cutover is judged on the rounded value, the same
  // one the string prints, so the two can never disagree.
  it("crosses over to miles once the rounded foot count reaches the mile, not before", () => {
    expect(formatDistance(1609.2, "imperial")).toBe("1.0 mi");
  });
});

describe("formatCount", () => {
  it("uses the singular for exactly one", () => {
    expect(formatCount(1, "point")).toBe("1 point");
  });

  it("uses the plural otherwise", () => {
    expect(formatCount(0, "point")).toBe("0 points");
    expect(formatCount(2, "point")).toBe("2 points");
  });
});

describe("formatTimestamp", () => {
  it("reports a missing timestamp plainly", () => {
    expect(formatTimestamp(undefined)).toBe("never");
  });

  it("reports an unparseable timestamp without throwing", () => {
    expect(formatTimestamp("not-a-date")).toBe("unknown");
  });

  it("renders a valid timestamp", () => {
    expect(formatTimestamp("2026-08-17T08:00:00Z")).not.toBe("unknown");
  });
});

describe("formatReadTime", () => {
  it("reports a library that has never been read", () => {
    expect(formatReadTime(undefined)).toBe("never");
  });

  it("reports an unparseable timestamp without throwing", () => {
    expect(formatReadTime("not-a-date")).toBe("unknown");
  });

  /*
   * The clock alone for today's read, which is nearly every read a card shows.
   * Asserted against what the platform itself would print rather than against a
   * literal, so the test says the same thing under every locale the suite runs
   * in — what is being checked is that the date was dropped, not the separator.
   */
  it("gives a read from today the clock alone", () => {
    const now = new Date("2026-08-17T19:38:00Z");
    const expected = now.toLocaleTimeString(undefined, { timeStyle: "short" });

    expect(formatReadTime(now.toISOString(), now)).toBe(expected);
  });

  // The one case where the short form would mislead: "19:38" on a library that
  // was last read on Sunday reads as a library read minutes ago.
  it("keeps the date on an older read", () => {
    const now = new Date("2026-08-17T19:38:00Z");
    const earlier = "2026-08-15T19:38:00Z";

    expect(formatReadTime(earlier, now)).toBe(formatTimestamp(earlier));
  });
});

describe("formatElevation", () => {
  /*
   * Not `formatDistance`: nought there means a route with nothing to measure and
   * reads as an em dash, but nought metres above sea level is the coast, and a
   * route that drops below it is a real one.
   */
  it.each([
    [0, "0 m"],
    [-4, "-4 m"],
    [960.4, "960 m"],
    [960.6, "961 m"],
  ])("formats %p as %p in metric", (metres, expected) => {
    expect(formatElevation(metres, "metric")).toBe(expected);
  });

  it("formats a height in feet for the imperial system", () => {
    expect(formatElevation(960.4, "imperial")).toBe("3,151 ft");
  });

  it("says nothing rather than NaN for a height it does not have", () => {
    expect(formatElevation(Number.NaN, "metric")).toBe("—");
    expect(formatElevation(Number.POSITIVE_INFINITY, "metric")).toBe("—");
  });
});

describe("formatAscent", () => {
  it("says nothing for a route with no usable elevation profile", () => {
    expect(formatAscent(0, "metric")).toBe("—");
    expect(formatAscent(-4, "metric")).toBe("—");
  });

  it("reports total climbing in metres for the metric system", () => {
    expect(formatAscent(2730, "metric")).toBe("2,730 m");
  });

  it("reports total climbing in feet for the imperial system", () => {
    expect(formatAscent(2730, "imperial")).toBe("8,957 ft");
  });
});

describe("formatGradient", () => {
  it("says nothing for a gradient too shallow to claim as one", () => {
    expect(formatGradient(0.5)).toBe("—");
    expect(formatGradient(Number.NaN)).toBe("—");
  });

  it("holds a decimal place under ten percent and drops it at ten and above", () => {
    expect(formatGradient(9.2)).toBe("9.2%");
    expect(formatGradient(11.6)).toBe("12%");
  });
});

describe("formatMovingTime", () => {
  it("says nothing for a time it does not have", () => {
    expect(formatMovingTime(undefined)).toBe("—");
    expect(formatMovingTime(0)).toBe("—");
    expect(formatMovingTime(-4)).toBe("—");
    expect(formatMovingTime(Number.NaN)).toBe("—");
  });

  it("rounds to the nearest five minutes under an hour", () => {
    expect(formatMovingTime(62)).toBe("5 min");
    expect(formatMovingTime(7 * 60 + 40)).toBe("10 min");
  });

  it("reports whole hours without a trailing zero", () => {
    expect(formatMovingTime(2 * 3600)).toBe("2 h");
  });

  it("reports hours and minutes together", () => {
    expect(formatMovingTime(2 * 3600 + 47 * 60)).toBe("2 h 45 min");
  });
});

describe("formatCadence", () => {
  it("says nothing about a schedule it does not have", () => {
    expect(formatCadence(undefined)).toBe("No fixed schedule");
    expect(formatCadence(0)).toBe("No fixed schedule");
    expect(formatCadence(-4)).toBe("No fixed schedule");
    expect(formatCadence(Number.NaN)).toBe("No fixed schedule");
  });

  it("names an hourly gap specially", () => {
    expect(formatCadence(3600)).toBe("Hourly");
  });

  it("reports a gap of whole hours as a count", () => {
    expect(formatCadence(6 * 3600)).toBe("Every 6 hours");
  });

  it("reports a gap under an hour in minutes", () => {
    expect(formatCadence(90)).toBe("Every 2 minutes");
    expect(formatCadence(60)).toBe("Every minute");
  });
});

describe("formatMovingTimeUncertainty", () => {
  it("is undefined when the loaded profile carries no measured result", () => {
    expect(formatMovingTimeUncertainty(undefined)).toBeUndefined();
  });

  it("reports the rounded mean absolute error", () => {
    expect(
      formatMovingTimeUncertainty({
        biasPercent: -1.2,
        maePercent: 6.8,
        p90Percent: 14.1,
        evaluatedRides: 42,
      }),
    ).toBe("±7% typical");
  });
});

describe("formatTemperature", () => {
  it("says nothing for a reading it does not have", () => {
    expect(formatTemperature(Number.NaN, "metric")).toBe("—");
    expect(formatTemperature(Number.POSITIVE_INFINITY, "metric")).toBe("—");
  });

  it("keeps a decimal near freezing, in metric", () => {
    expect(formatTemperature(0.4, "metric")).toBe("0.4°C");
  });

  it("drops the decimal once safely away from freezing, in metric", () => {
    expect(formatTemperature(18.2, "metric")).toBe("18°C");
  });

  it("converts to Fahrenheit for the imperial system", () => {
    expect(formatTemperature(18.2, "imperial")).toBe("65°F");
  });

  /*
   * Freezing is 0 on one scale and 32 on the other, so a threshold applied to
   * the converted number keeps the decimal in the wrong places: it would drop
   * it at freezing point — the one reading where the digit decides between
   * rain and ice — and hand it back on a hard frost at −10°C, which reads 14°F.
   */
  it("keeps the decimal near freezing in imperial too, and drops it far from it", () => {
    expect(formatTemperature(0.4, "imperial")).toBe("32.7°F");
    expect(formatTemperature(-18, "imperial")).toBe("0°F");
  });
});

describe("formatWindSpeed", () => {
  it("says nothing for a reading it does not have", () => {
    expect(formatWindSpeed(Number.NaN, "metric")).toBe("—");
  });

  it("holds a decimal under ten and drops it at ten and above, in metric", () => {
    expect(formatWindSpeed(4.2, "metric")).toBe("4.2 km/h");
    expect(formatWindSpeed(18, "metric")).toBe("18 km/h");
  });

  it("converts to miles per hour for the imperial system", () => {
    expect(formatWindSpeed(18, "imperial")).toBe("11 mph");
    expect(formatWindSpeed(3, "imperial")).toBe("1.9 mph");
  });
});

describe("formatPrecipitation", () => {
  it("says nothing for a reading it does not have", () => {
    expect(formatPrecipitation(Number.NaN, "metric")).toBe("—");
  });

  it("reports one decimal of millimetres in metric", () => {
    expect(formatPrecipitation(0.4, "metric")).toBe("0.4 mm");
  });

  // An inch is roughly twenty-five millimetres, so the same one-decimal
  // precision would round a light shower away to nothing in inches — two
  // decimals keeps the same real resolution as one decimal of millimetres.
  it("reports two decimals of inches in imperial", () => {
    expect(formatPrecipitation(25.4, "imperial")).toBe("1.00 in");
    expect(formatPrecipitation(0.4, "imperial")).toBe("0.02 in");
  });
});
