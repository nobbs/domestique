/**
 * Nine weeks of synthetic rides for the index spikes: enough of them, varied
 * enough, that a layout has to cope with a rest week, a rainy ride, and a ride
 * whose bicycle carried no strap. Generated from a fixed seed so every story
 * shows the same rides.
 */

import type { Activity, Position } from "../../../api/types";

export interface IndexRide extends Activity {
  /** The shape the glyph draws. */
  coordinates: Position[];
  /** The steepest band the glyph is stroked in. */
  band: number;
}

function lcg(seed: number): () => number {
  let state = seed;
  return () => {
    state = (state * 1_664_525 + 1_013_904_223) % 4_294_967_296;
    return state / 4_294_967_296;
  };
}

function loop(random: () => number): Position[] {
  const a = 3 + Math.floor(random() * 4);
  const b = 5 + Math.floor(random() * 6);
  const wa = 0.05 + random() * 0.15;
  const wb = 0.03 + random() * 0.08;
  return Array.from({ length: 80 }, (_, index) => {
    const theta = (index / 79) * Math.PI * 2;
    const wobble = 1 + wa * Math.sin(a * theta) + wb * Math.cos(b * theta);
    return [8 + 0.1 * wobble * Math.cos(theta), 48 + 0.07 * wobble * Math.sin(theta)];
  });
}

/** Sunday 6 Sept 2026 is the last day shown; rides count back from it. */
const LAST_DAY = Date.UTC(2026, 8, 6);
const DAY = 86_400_000;

function build(): IndexRide[] {
  const random = lcg(7);
  const rides: IndexRide[] = [];
  let id = 200;
  for (let day = 0; day < 63; day += 1) {
    const weekday = (day + 1) % 7;
    const restWeek = day >= 21 && day < 28;
    const chance = restWeek ? 0.1 : weekday === 5 || weekday === 6 ? 0.8 : 0.35;
    if (random() > chance) {
      continue;
    }
    const long = weekday === 6 && random() > 0.4;
    const distanceMetres = Math.round((long ? 70 + random() * 60 : 18 + random() * 45) * 1000);
    const hilly = random();
    const ascentMetres = Math.round((distanceMetres / 1000) * (2 + hilly * 26));
    const speedKmh = 26 - hilly * 9 + random() * 2;
    const movingSeconds = Math.round((distanceMetres / 1000 / speedKmh) * 3600);
    const strap = random() > 0.2;
    const temperature = 9 + random() * 16;
    const rain = random() > 0.8 ? Math.round(random() * 80) / 10 : 0;
    const hour = 7 + Math.floor(random() * 10);
    id += 1;
    rides.push({
      id,
      startedAt: new Date(LAST_DAY - (62 - day) * DAY + hour * 3_600_000).toISOString(),
      distanceMetres,
      movingSeconds,
      elapsedSeconds: movingSeconds + Math.round(random() * 1800),
      ascentMetres,
      typeId: 1,
      locationId: 1,
      provider: "wahoo",
      ...(strap
        ? {
            metrics: {
              averageHeartRateBpm: Math.round(120 + hilly * 30 + random() * 10),
              maxHeartRateBpm: Math.round(160 + hilly * 15),
              heartRateTss: Math.round((movingSeconds / 3600) * (35 + hilly * 45)),
              trimp: Math.round((movingSeconds / 3600) * (40 + hilly * 40)),
            },
          }
        : {}),
      weather: {
        temperatureMinCelsius: temperature,
        temperatureMaxCelsius: temperature + 2 + random() * 6,
        windSpeedKmh: Math.round(5 + random() * 25),
        precipitationMillimetres: rain,
        weatherCode: rain > 0 ? 61 : random() > 0.5 ? 2 : 0,
      },
      coordinates: loop(random),
      band: Math.min(4, Math.floor(hilly * 5)),
    });
  }
  return rides.reverse();
}

/** Newest first, as the list reads. */
export const RIDES: IndexRide[] = build();
