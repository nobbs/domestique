import { describe, expect, it } from "vitest";
import type { MeasureKey } from "./measures";
import {
  bandRange,
  MEASURES,
  WIND_RELATION_KEY,
  windRelationColour,
  windRelationStop,
  windRelationVariable,
  windRelationWords,
} from "./measures";

function measure(key: MeasureKey) {
  const found = MEASURES.find((entry) => entry.key === key);
  if (!found) {
    throw new Error(`no measure registered for ${key}`);
  }

  return found;
}

const HEX = /^#[0-9a-f]{6}$/i;

describe("MEASURES", () => {
  it("lists all four measures once each, in a stable order", () => {
    expect(MEASURES.map((entry) => entry.key)).toEqual(["wind", "temperature", "rain", "cloud"]);
  });

  it("bands wind by strength", () => {
    const { band } = measure("wind");
    expect(band(14.9)).toBe(0);
    expect(band(15)).toBe(1);
    expect(band(29.9)).toBe(1);
    expect(band(30)).toBe(2);
    expect(band(44.9)).toBe(2);
    expect(band(45)).toBe(3);
  });

  it("bands temperature at the same floors weather.ts cuts the jersey at", () => {
    const { band } = measure("temperature");
    expect(band(4.9)).toBe(0);
    expect(band(5)).toBe(1);
    expect(band(11.9)).toBe(1);
    expect(band(12)).toBe(2);
    expect(band(19.9)).toBe(2);
    expect(band(20)).toBe(3);
    expect(band(26.9)).toBe(3);
    expect(band(27)).toBe(4);
  });

  it("bands rain from dry through heavy", () => {
    const { band } = measure("rain");
    expect(band(0.1)).toBe(0);
    expect(band(0.2)).toBe(1);
    expect(band(1.9)).toBe(1);
    expect(band(2)).toBe(2);
    expect(band(5.9)).toBe(2);
    expect(band(6)).toBe(3);
  });

  it("bands cloud from clear through overcast", () => {
    const { band } = measure("cloud");
    expect(band(19)).toBe(0);
    expect(band(20)).toBe(1);
    expect(band(49)).toBe(1);
    expect(band(50)).toBe(2);
    expect(band(84)).toBe(2);
    expect(band(85)).toBe(3);
  });

  it("leaves rain and cloud's lowest band with nothing to say", () => {
    expect(measure("rain").opacity(0)).toBe(0);
    expect(measure("cloud").opacity(0)).toBe(0);
    for (const band of [1, 2, 3]) {
      expect(measure("rain").opacity(band)).toBe(1);
      expect(measure("cloud").opacity(band)).toBe(1);
    }
  });

  it("never leaves wind or temperature unpainted", () => {
    for (const band of [0, 1, 2, 3]) {
      expect(measure("wind").opacity(band)).toBe(1);
    }
    for (const band of [0, 1, 2, 3, 4]) {
      expect(measure("temperature").opacity(band)).toBe(1);
    }
  });

  it("colours every band it has, on both grounds", () => {
    for (const entry of MEASURES) {
      entry.bands.forEach((_band, index) => {
        expect(entry.colour(index, false)).toMatch(HEX);
        expect(entry.colour(index, true)).toMatch(HEX);
      });
    }
  });

  it("still describes the dry and clear bands in words, despite painting nothing", () => {
    expect(measure("rain").words(0.1)).toContain("dry");
    expect(measure("cloud").words(10)).toContain("clear");
  });

  it("names each measure's unit in its words", () => {
    expect(measure("wind").words(25)).toContain("km/h");
    expect(measure("temperature").words(3)).toContain("°C");
    expect(measure("rain").words(3)).toContain("mm");
    expect(measure("cloud").words(60)).toContain("60%");
  });

  it("reads temperature apparent rather than actual, and wind and rain and cloud their own field", () => {
    const point = {
      time: "2026-09-03T12:00:00Z",
      temperatureCelsius: 30,
      apparentTemperatureCelsius: 18,
      precipitationMillimetres: 4,
      precipitationProbabilityPercent: 50,
      windSpeedKmh: 22,
      windDirectionDegrees: 270,
      weatherCode: 61,
      cloudCoverPercent: 70,
    };

    expect(measure("temperature").reading(point)).toBe(18);
    expect(measure("wind").reading(point)).toBe(22);
    expect(measure("rain").reading(point)).toBe(4);
    expect(measure("cloud").reading(point)).toBe(70);
  });
});

/**
 * The second ramp the wind measure carries: not how hard it blows but which way
 * it sits against the road, which is what the route itself is drawn in.
 */
describe("the head-to-tail ramp", () => {
  it("puts a headwind at one end and a tailwind at the other", () => {
    expect(windRelationStop("head", -1)).toBe(0);
    expect(windRelationStop("tail", 1)).toBe(3);
  });

  it("leans a crosswind to whichever side its component points", () => {
    expect(windRelationStop("cross", -0.5)).toBe(1);
    expect(windRelationStop("cross", 0.5)).toBe(2);
  });

  it("colours every stop, and `mixed` in something that is on neither end", () => {
    for (const dark of [false, true]) {
      const neutral = windRelationColour(null, dark);
      expect(neutral).toMatch(HEX);
      for (const stop of [0, 1, 2, 3]) {
        expect(windRelationColour(stop, dark)).toMatch(HEX);
        expect(windRelationColour(stop, dark)).not.toBe(neutral);
      }
    }
  });

  it("gives each stop a colour of its own, so no two stretches read alike", () => {
    const drawn = [0, 1, 2, 3, null].map((stop) => windRelationColour(stop, false));

    expect(new Set(drawn).size).toBe(drawn.length);
  });

  it("names the same stops in the custom properties a legend reads", () => {
    expect(windRelationVariable(0)).toBe("var(--wind-relation-0)");
    expect(windRelationVariable(null)).toBe("var(--wind-relation-mixed)");
  });

  it("lists every stop in the key, the neutral last", () => {
    expect(WIND_RELATION_KEY.map((band) => band.stop)).toEqual([0, 1, 2, 3, null]);
    for (const band of WIND_RELATION_KEY) {
      expect(band.description).not.toBe("");
    }
  });

  it("says which way the wind sits in words", () => {
    expect(windRelationWords(0, 24)).toBe("headwind, 24 km/h");
    expect(windRelationWords(3, 24)).toBe("tailwind, 24 km/h");
    expect(windRelationWords(1, 24)).toContain("crosswind");
    expect(windRelationWords(null, 24)).toContain("wind shifting");
  });
});

describe("bandRange", () => {
  it("gives wind's cuts in km/h", () => {
    const wind = measure("wind");

    expect(bandRange(wind, 0)).toBe("under 15 km/h");
    expect(bandRange(wind, 1)).toBe("15–30 km/h");
    expect(bandRange(wind, 2)).toBe("30–45 km/h");
    expect(bandRange(wind, 3)).toBe("over 45 km/h");
  });

  it("gives temperature's cuts in °C, coldest band first", () => {
    const temperature = measure("temperature");

    expect(bandRange(temperature, 0)).toBe("under 5 °C");
    expect(bandRange(temperature, 1)).toBe("5–12 °C");
    expect(bandRange(temperature, 2)).toBe("12–20 °C");
    expect(bandRange(temperature, 3)).toBe("20–27 °C");
    expect(bandRange(temperature, 4)).toBe("over 27 °C");
  });

  it("gives rain's cuts in mm/h", () => {
    const rain = measure("rain");

    expect(bandRange(rain, 0)).toBe("under 0.2 mm/h");
    expect(bandRange(rain, 1)).toBe("0.2–2 mm/h");
    expect(bandRange(rain, 2)).toBe("2–6 mm/h");
    expect(bandRange(rain, 3)).toBe("over 6 mm/h");
  });

  it("gives cloud's cuts as a percentage", () => {
    const cloud = measure("cloud");

    expect(bandRange(cloud, 0)).toBe("under 20 %");
    expect(bandRange(cloud, 1)).toBe("20–50 %");
    expect(bandRange(cloud, 2)).toBe("50–85 %");
    expect(bandRange(cloud, 3)).toBe("over 85 %");
  });
});
