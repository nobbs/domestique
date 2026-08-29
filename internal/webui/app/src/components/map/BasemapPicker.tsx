/**
 * The ground under the library, chosen by the reader.
 *
 * The operator names the basemaps a deployment offers; which of them is on
 * screen is a matter of what the reader is trying to see. Streets answer where
 * a road goes and imagery answers what the ground there actually is, and no
 * operator can pick between those two questions once, in a config file, on
 * somebody else's behalf.
 *
 * A mark in the map's own control cluster, folded away until it is asked for,
 * for the same reason the credit beside it folds: the map is the page, and the
 * furniture on it earns its room. Nothing is shown at all where the operator
 * configured one basemap — one basemap is not a choice, and a control that can
 * only confirm what is already true is noise in the corner.
 *
 * It opens over the map rather than growing inside the cluster. Unfolded in
 * place, the choices wrapped across a slab as wide as the corner and shoved the
 * mark that opened them sideways; a popover leaves the cluster the size it was,
 * stacks the names one to a line however many the operator configured, and
 * closes to Escape and to a click on the map — which is what a reader who has
 * seen enough reaches for.
 *
 * Which entry is checked is handed in rather than worked out here, because the
 * fallbacks in `basemap.ts` decide it: a reader who has picked nothing, or
 * picked an entry since removed from the config, is looking at a basemap they
 * did not name, and the mark must follow the ground actually loaded.
 */

import { IconStack2 } from "@tabler/icons-react";
import { Button } from "@/components/Button";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { Basemap } from "../../api/types";
import { BasemapPreview } from "./BasemapPreview";

/**
 * Past this many, the names stand on their own.
 *
 * Every preview is a WebGL context and a browser hands out about sixteen for
 * the whole page; the map behind this one has already spent one. An operator
 * offering a dozen basemaps gets a list rather than a gallery, which is a worse
 * list but a working page.
 */
const MOST_PREVIEWS = 8;

export interface BasemapPickerProps {
  /** Every basemap the operator offers, in the order they configured them. */
  basemaps: Basemap[];
  /** The one on screen, by name. */
  selectedName: string;
  onSelect: (name: string) => void;
  /** Whether the reader has the list open. Held by the caller — see below. */
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

export function BasemapPicker({
  basemaps,
  selectedName,
  onSelect,
  expanded,
  onExpandedChange,
}: BasemapPickerProps) {
  if (basemaps.length < 2) {
    return null;
  }

  const showPreviews = basemaps.length <= MOST_PREVIEWS;

  return (
    <Popover open={expanded} onOpenChange={onExpandedChange}>
      <PopoverTrigger
        render={<Button variant="panel" icon={<IconStack2 stroke={1.6} />} />}
        // The mark says "the ground is a choice" to anyone who can see it; the name
        // says what, for anyone who cannot. The name has to carry the fold, since
        // `aria-expanded` alone leaves a reader guessing what pressing it does.
        aria-label={expanded ? "Hide the basemap choices" : "Choose the basemap"}
      />
      {/*
       * Downwards and towards the corner it is anchored in: the control sits
       * at the head of the map's own stack, under the zoom pair.
       */}
      <PopoverContent
        align="end"
        // The popup is a dialog to anything that reads rather than looks, and a
        // dialog with no name is a room with no sign on the door.
        aria-label="Basemap choices"
        className="w-auto gap-0 bg-[var(--panel)] p-1 shadow-[var(--shadow)]"
        side="bottom"
      >
        <RadioGroup
          aria-label="Basemap"
          className="grid w-auto gap-0"
          onValueChange={(value) => onSelect(String(value))}
          value={selectedName}
        >
          {basemaps.map((basemap, index) => {
            const itemID = `map-basemap-${index}`;

            /*
             * The whole row is the target, not the dot beside the name: a
             * reader aiming at a list of grounds is aiming at the name they
             * can read, and the label is what makes the rest of the row count
             * as pressing it.
             *
             * The picture says how a basemap looks and the name says which one
             * it is. Neither stands in for the other — the picture is hidden
             * from anything that reads rather than looks, and over open country
             * it cannot tell two light styles apart even for those who can see
             * it.
             */
            return (
              <Label
                className="cursor-pointer gap-3 rounded-md p-1.5 font-normal hover:bg-[var(--base)] has-[:focus-visible]:bg-[var(--base)]"
                htmlFor={itemID}
                key={basemap.name}
              >
                {/*
                 * Hidden while a picture is carrying the state, because the
                 * tile's accent edge already says which ground is on screen
                 * and a dot beside it would say it twice. Hidden by a wrapper
                 * rather than by a class on the control: the primitive's own
                 * `relative size-4` outranks `sr-only`, which left it invisible
                 * but still holding a column of the row open.
                 *
                 * Without pictures the dot comes back: something visible has to
                 * mark the choice, and the row's focus treatment is not that.
                 */}
                {showPreviews ? (
                  <span className="sr-only">
                    <RadioGroupItem id={itemID} value={basemap.name} />
                  </span>
                ) : (
                  <RadioGroupItem id={itemID} value={basemap.name} />
                )}
                {showPreviews ? (
                  <BasemapPreview
                    selected={basemap.name === selectedName}
                    styleUrl={basemap.styleUrl}
                  />
                ) : null}
                {basemap.name}
              </Label>
            );
          })}
        </RadioGroup>
      </PopoverContent>
    </Popover>
  );
}
