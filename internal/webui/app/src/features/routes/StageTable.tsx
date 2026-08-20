/**
 * The route library as rows: the same stages the grid shows, read as figures
 * rather than as pictures.
 *
 * It carries no previews, and that is the point of it — comparing six stages by
 * climbing means reading six numbers in a column, which a grid of thumbnails
 * makes into six separate glances. It is presentational in the same way
 * `RouteCard` is: it takes the stages the page already arranged and knows
 * nothing about fetching, so the search and the order stay the page's.
 *
 * The column headers say what the cards say in words, rather than the shorthand
 * a narrow column invites: the same figure must not be called two things
 * depending on which presentation it is being read in.
 *
 * The name is the row's header cell, so a screen reader announces which stage a
 * figure belongs to when reading across, and the link sits in it rather than
 * around the row: a row is not a link, and wrapping one in an anchor would make
 * every figure part of the link's name.
 */

import { Link } from "react-router";
import type { Stage } from "../../api/types";
import { stageKey } from "../../api/types";
import { formatAscent, formatDistance, formatGradient } from "../../lib/format";
import styles from "./StageTable.module.css";

export function StageTable({ stages }: { stages: Stage[] }) {
  return (
    <table className={styles.table}>
      <caption className="visually-hidden">Route library</caption>
      <thead>
        <tr>
          <th scope="col">Stage</th>
          <th scope="col">Distance</th>
          <th scope="col">Total climbing</th>
          <th scope="col">Steepest sustained gradient</th>
        </tr>
      </thead>
      <tbody>
        {stages.map((stage) => (
          <tr key={stageKey(stage)}>
            <th scope="row" className={styles.name}>
              <Link to={`/routes/${stage.routeId}/${stage.stageOrder}`}>{stage.title}</Link>
            </th>
            <td className={styles.figure}>{formatDistance(stage.distanceMetres)}</td>
            <td className={styles.figure}>{formatAscent(stage.ascentMetres)}</td>
            <td className={styles.figure}>{formatGradient(stage.maxGradientPercent)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
