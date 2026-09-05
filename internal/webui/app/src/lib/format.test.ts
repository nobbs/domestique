import { describe, expect, it } from "vitest";
import {
  formatAscent,
  formatCadence,
  formatCount,
  formatDescent,
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
  ])("formats %p as %p", (metres, expected) => {
    expect(formatDistance(metres)).toBe(expected);
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
  ])("formats %p as %p", (metres, expected) => {
    expect(formatElevation(metres)).toBe(expected);
  });

  it("says nothing rather than NaN for a height it does not have", () => {
    expect(formatElevation(Number.NaN)).toBe("—");
    expect(formatElevation(Number.POSITIVE_INFINITY)).toBe("—");
  });
});

describe("formatAscent", () => {
  it("says nothing for a route with no usable elevation profile", () => {
    expect(formatAscent(0)).toBe("—");
    expect(formatAscent(-4)).toBe("—");
  });

  it("reports total climbing in metres", () => {
    expect(formatAscent(2730)).toBe("2,730 m");
  });
});

describe("formatDescent", () => {
  it("says nothing for a route that only climbs, or has no usable elevation profile", () => {
    expect(formatDescent(0)).toBe("—");
    expect(formatDescent(-4)).toBe("—");
  });

  it("reports total descending in metres", () => {
    expect(formatDescent(2690)).toBe("2,690 m");
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

  it("reports a gap under a minute in seconds rather than rounding it away", () => {
    expect(formatCadence(30)).toBe("Every 30 seconds");
    expect(formatCadence(1)).toBe("Every second");
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
    expect(formatTemperature(Number.NaN)).toBe("—");
    expect(formatTemperature(Number.POSITIVE_INFINITY)).toBe("—");
  });

  it("keeps a decimal near freezing", () => {
    expect(formatTemperature(0.4)).toBe("0.4°C");
  });

  it("drops the decimal once safely away from freezing", () => {
    expect(formatTemperature(18.2)).toBe("18°C");
  });
});

describe("formatWindSpeed", () => {
  it("says nothing for a reading it does not have", () => {
    expect(formatWindSpeed(Number.NaN)).toBe("—");
  });

  it("holds a decimal under ten and drops it at ten and above", () => {
    expect(formatWindSpeed(4.2)).toBe("4.2 km/h");
    expect(formatWindSpeed(18)).toBe("18 km/h");
  });
});

describe("formatPrecipitation", () => {
  it("says nothing for a reading it does not have", () => {
    expect(formatPrecipitation(Number.NaN)).toBe("—");
  });

  it("reports one decimal of millimetres", () => {
    expect(formatPrecipitation(0.4)).toBe("0.4 mm");
  });
});
