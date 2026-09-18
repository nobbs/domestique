/**
 * One place on a page that can show the same subject more than one way: a
 * heading, a switch between the ways, and whichever one is chosen.
 *
 * A view's content is rendered only while it is chosen, so a view that has to
 * fetch what it draws asks for nothing until the reader picks it.
 */

import { type ReactNode, useState } from "react";
import { Segmented } from "./Segmented";

export interface View<V extends string> {
  value: V;
  label: string;
  content: () => ReactNode;
}

export interface SwitchableViewProps<V extends string> {
  /** Names the switch for assistive technology; the heading is not always text. */
  label: string;
  heading?: ReactNode;
  views: readonly View<V>[];
  /** The first view when absent. */
  defaultValue?: V;
  onValueChange?: (value: V) => void;
}

/** A single view is shown with no switch at all: there is nothing to choose between. */
export function SwitchableView<V extends string>({
  label,
  heading,
  views,
  defaultValue,
  onValueChange,
}: SwitchableViewProps<V>) {
  const [chosen, setChosen] = useState<V | undefined>(defaultValue);
  const current = views.find((view) => view.value === chosen) ?? views[0];
  if (!current) {
    return null;
  }

  return (
    <div className="flex flex-col gap-4">
      {heading !== undefined || views.length > 1 ? (
        <div className="flex min-h-7 items-center justify-between gap-3">
          {heading ?? <span />}
          {views.length > 1 ? (
            <Segmented
              label={label}
              size="sm"
              items={views.map((view) => ({ key: view.value, label: view.label }))}
              value={current.value}
              onChange={(next) => {
                setChosen(next);
                onValueChange?.(next);
              }}
            />
          ) : null}
        </div>
      ) : null}
      {current.content()}
    </div>
  );
}
