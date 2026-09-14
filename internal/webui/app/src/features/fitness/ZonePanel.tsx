/** Each week's time in heart-rate zones, stacked easiest at the foot, on the ride page's colours. */

import type { FitnessWeek } from "../../api/types";
import { ChartLegend, ReadoutRow, TimeFrame } from "../../components/chart/TimeFrame";
import { formatDuration } from "../../lib/format";
import { ZONE_NAMES, zoneColour } from "../activity/HeartRateZones";
import { daysBetween } from "./form";

interface Props {
  weeks: readonly FitnessWeek[];
  dates: readonly string[];
}

export function ZonePanel({ weeks, dates }: Props) {
  const firstDay = dates[0] ?? "";
  const lastIndex = dates.length - 1;
  // A week that began before the range is drawn over the days of it inside the range.
  const span = (week: FitnessWeek) => {
    const start = daysBetween(firstDay, week.weekStart);
    return { start: Math.max(start, 0), end: Math.min(start + 7, lastIndex) };
  };
  const shown = weeks.filter((week) => {
    const { start, end } = span(week);
    return end > start && start <= lastIndex;
  });
  if (shown.length === 0) {
    return null;
  }
  const hours = (week: FitnessWeek) =>
    week.zoneSeconds.reduce((sum, seconds) => sum + seconds, 0) / 3600;
  const high = Math.max(1, ...shown.map(hours));

  return (
    <>
      <ChartLegend
        items={ZONE_NAMES.map((name, zone) => ({
          label: name,
          colour: zoneColour(zone),
          swatch: true,
        }))}
      />
      <TimeFrame
        label={`Hours in each heart-rate zone over ${shown.length} ${shown.length === 1 ? "week" : "weeks"}`}
        dates={dates}
        snap={shown.map((week) => span(week).start)}
        readout={(index) => {
          const week = shown.find((one) => span(one).start === index);
          if (!week) {
            return null;
          }
          return (
            <>
              <ReadoutRow label="in total" value={formatDuration(hours(week) * 3600)} />
              {ZONE_NAMES.map((name, zone) => ({ name, zone }))
                .reverse()
                .map(({ name, zone }) => (
                  <ReadoutRow
                    key={name}
                    colour={zoneColour(zone)}
                    label={name}
                    value={formatDuration(week.zoneSeconds[zone] ?? 0)}
                  />
                ))}
            </>
          );
        }}
        panels={[
          {
            height: 150,
            domain: [0, high],
            format: (value) => `${value}h`,
            draw: (x, y) =>
              shown.map((week) => {
                const { start, end } = span(week);
                const left = x(start) + 1;
                const width = Math.max(x(end) - left - 1, 1);
                let base = 0;
                return (
                  <g key={week.weekStart}>
                    {week.zoneSeconds.map((seconds, zone) => {
                      const top = base + seconds / 3600;
                      const segment = (
                        <rect
                          key={ZONE_NAMES[zone]}
                          x={left}
                          y={y(top)}
                          width={width}
                          // A surface-coloured gap keeps neighbouring zones apart.
                          height={Math.max(y(base) - y(top) - 1, 0)}
                          fill={zoneColour(zone)}
                        />
                      );
                      base = top;
                      return segment;
                    })}
                  </g>
                );
              }),
          },
        ]}
      />
    </>
  );
}
