/**
 * What the map shows over the whole area, and at what hour.
 *
 * One popover rather than a button per measure: the toggles are not mutually
 * exclusive — any combination can be on together — and a row of pressed
 * buttons in the corner cannot also hold the time scale a reader needs once
 * more than one is on. Follows `BasemapPicker`'s shape: a mark in the map's
 * own control cluster, folded away until asked for.
 *
 * The clock lives here and nowhere else. Every overlay reads the same
 * `hoursAhead`, so switching two of them on and scrubbing the hour never
 * leaves one measure a step out of sync with another.
 */

import type { IconProps } from "@tabler/icons-react";
import { IconCloud, IconCloudRain, IconTemperature, IconWind } from "@tabler/icons-react";
import type { ComponentType } from "react";
import { Button } from "@/components/Button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import type { MeasureKey } from "../../lib/measures";

/** The forecast horizon ICON-D2 publishes past its reference run. */
export const MAX_HOURS_AHEAD = 48;

const MEASURE_ICON: Record<MeasureKey, ComponentType<IconProps>> = {
  wind: IconWind,
  temperature: IconTemperature,
  rain: IconCloudRain,
  cloud: IconCloud,
};

/** What the hour scale reads at zero and past it, matching `hoursAhead`'s own units. */
function hourLabel(hoursAhead: number): string {
  if (hoursAhead === 0) {
    return "Now";
  }
  const at = new Date(Math.round(Date.now() / 3_600_000 + hoursAhead) * 3_600_000);

  return `+${hoursAhead}h · ${at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}`;
}

export interface WeatherOverlayPickerProps {
  measures: readonly { key: MeasureKey; label: string }[];
  selected: ReadonlySet<MeasureKey>;
  onToggle: (key: MeasureKey, on: boolean) => void;
  hoursAhead: number;
  onHoursAheadChange: (hours: number) => void;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

export function WeatherOverlayPicker({
  measures,
  selected,
  onToggle,
  hoursAhead,
  onHoursAheadChange,
  expanded,
  onExpandedChange,
}: WeatherOverlayPickerProps) {
  const anyOn = selected.size > 0;

  return (
    <Popover open={expanded} onOpenChange={onExpandedChange}>
      <PopoverTrigger
        render={<Button variant="panel" active={anyOn} icon={<IconCloud stroke={1.6} />} />}
        aria-label={expanded ? "Hide the weather overlay choices" : "Show weather over the map"}
      />
      <PopoverContent
        align="end"
        aria-label="Weather overlay choices"
        className="w-56 gap-3 bg-[var(--panel)] p-3 shadow-[var(--shadow)]"
        side="bottom"
      >
        <div className="grid gap-1">
          {measures.map((measure) => {
            const Icon = MEASURE_ICON[measure.key];
            const itemID = `map-overlay-${measure.key}`;

            return (
              <Label
                className="flex cursor-pointer items-center gap-2 rounded-md p-1.5 font-normal hover:bg-[var(--base)] has-[:focus-visible]:bg-[var(--base)]"
                htmlFor={itemID}
                key={measure.key}
              >
                <Checkbox
                  id={itemID}
                  checked={selected.has(measure.key)}
                  onCheckedChange={(checked) => onToggle(measure.key, checked === true)}
                />
                <Icon stroke={1.6} className="size-4 text-[var(--ink-2)]" aria-hidden="true" />
                {measure.label}
              </Label>
            );
          })}
        </div>
        {/* The clock only matters once something is drawing from it. */}
        {anyOn ? (
          <div className="grid gap-1.5 border-[var(--rule)] border-t pt-3">
            <Label htmlFor="map-overlay-hour" className="flex justify-between font-normal text-xs">
              <span>When</span>
              <span className="text-[var(--ink-2)]">{hourLabel(hoursAhead)}</span>
            </Label>
            <Slider
              id="map-overlay-hour"
              min={0}
              max={MAX_HOURS_AHEAD}
              step={1}
              value={hoursAhead}
              onValueChange={(value) =>
                onHoursAheadChange(Array.isArray(value) ? (value[0] ?? 0) : value)
              }
            />
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
