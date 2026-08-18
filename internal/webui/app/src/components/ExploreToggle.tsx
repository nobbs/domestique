/**
 * The control that hands the map its gestures, and takes them back.
 *
 * A map is a picture until someone asks it a question, and asking costs the
 * page the wheel and the fingers over everything the canvas covers. This is
 * where that trade is made, once and visibly, instead of on every gesture: the
 * reader presses it to explore and presses it again — or presses Escape — to go
 * back to reading. Nothing is printed over the cartography either way.
 *
 * The label stays the same in both states and the pressed styling carries which
 * one it is in, because a control whose visible words change under a screen
 * reader that has already been told it is pressed says the same thing twice and
 * disagrees with itself in between. What the pressed state cannot say in words —
 * that the page has stopped scrolling here, and how to get that back — the note
 * beside it says, and says out loud: it is a live region, so entering
 * exploration is announced rather than merely rendered.
 */

import { Toggle } from "radix-ui";
import styles from "./ExploreToggle.module.css";

/** Said once, when the map takes the gestures, and read out where it appears. */
const EXPLORING_NOTE = "Exploring the map · Escape to leave";

export interface ExploreToggleProps {
  exploring: boolean;
  onExploringChange: (exploring: boolean) => void;
}

export function ExploreToggle({ exploring, onExploringChange }: ExploreToggleProps) {
  return (
    <div className={styles.control}>
      <Toggle.Root
        className={styles.toggle}
        pressed={exploring}
        onPressedChange={onExploringChange}
        // Only true of the state it is in: Escape leaves exploration, and a
        // control that claimed the key while the page was being read would be
        // promising something the page has no use for.
        aria-keyshortcuts={exploring ? "Escape" : undefined}
      >
        Explore map
      </Toggle.Root>
      {/*
       * Mounted in both states so the region exists before it has anything to
       * say. A live region added to the page already full is one an assistive
       * technology has no change to report.
       */}
      <p className={styles.note} aria-live="polite">
        {exploring ? EXPLORING_NOTE : ""}
      </p>
    </div>
  );
}
