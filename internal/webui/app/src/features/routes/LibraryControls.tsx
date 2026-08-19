/**
 * The controls above the library grid: what to look for, and what order to read
 * it in.
 *
 * Both are held by the page rather than here, because both describe what the
 * grid below is showing and the grid is the page's. This is the row of controls
 * and the sentence saying what they left, and nothing else.
 *
 * The search is a landmark, so a reader who arrives by keyboard reaches it
 * without walking the whole header, and the count beside it is polite rather
 * than assertive: typing into a search box is not an event worth interrupting a
 * screen reader mid-word for, but the result of it is worth hearing once the
 * typing stops.
 */

import { useId } from "react";
import type { StageSort } from "../../lib/library";
import { STAGE_SORT_LABELS, STAGE_SORTS } from "../../lib/library";
import styles from "./LibraryControls.module.css";

export interface LibraryControlsProps {
  query: string;
  onQueryChange: (query: string) => void;
  sort: StageSort;
  onSortChange: (sort: StageSort) => void;
  /** How many stages the grid is showing, and how many the library holds. */
  shown: number;
  total: number;
}

/**
 * What the controls left, in words.
 *
 * Silent until a search narrows something: "6 of 6 stages" beside an untouched
 * search box is a sentence that says only that nothing has happened yet.
 */
function countLabel(shown: number, total: number): string | null {
  if (shown === total) {
    return null;
  }

  return `Showing ${shown} of ${total} stages`;
}

export function LibraryControls({
  query,
  onQueryChange,
  sort,
  onSortChange,
  shown,
  total,
}: LibraryControlsProps) {
  const searchId = useId();
  const sortId = useId();
  const count = countLabel(shown, total);

  return (
    <search className={styles.controls} aria-label="Route library">
      <div className={styles.field}>
        <label className={styles.label} htmlFor={searchId}>
          Search
        </label>
        <input
          id={searchId}
          className={styles.search}
          type="search"
          value={query}
          placeholder="Route or stage name"
          autoComplete="off"
          onChange={(event) => onQueryChange(event.target.value)}
        />
      </div>
      <div className={styles.field}>
        <label className={styles.label} htmlFor={sortId}>
          Sort by
        </label>
        <select
          id={sortId}
          className={styles.sort}
          value={sort}
          // Read back out of the offered orders rather than cast: an order the
          // control never listed cannot become the one the grid sorts by.
          onChange={(event) => {
            const picked = STAGE_SORTS.find((option) => option === event.target.value);
            if (picked) {
              onSortChange(picked);
            }
          }}
        >
          {STAGE_SORTS.map((option) => (
            <option key={option} value={option}>
              {STAGE_SORT_LABELS[option]}
            </option>
          ))}
        </select>
      </div>
      <p className={styles.count} aria-live="polite">
        {count}
      </p>
    </search>
  );
}
