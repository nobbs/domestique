import { FieldLegend, FieldSet } from "../../components/ui/field";
import type { NumericRange } from "../../lib/filters";
import { BoundInput } from "./BoundInput";

export interface RangeRowProps {
  legend: string;
  unit: string;
  range: NumericRange;
  onChange: (next: NumericRange) => void;
  /** Stored-unit (metres, percent) to and from what the field shows. */
  toDisplay?: (stored: number) => number;
  toStored?: (display: number) => number;
}

/** One metric's min and max, both inclusive, both optional. */
export function RangeRow({
  legend,
  unit,
  range,
  onChange,
  toDisplay = (value) => value,
  toStored = (value) => value,
}: RangeRowProps) {
  return (
    <FieldSet className="grid grid-cols-2 gap-2">
      <FieldLegend variant="label" className="col-span-2 font-semibold">
        {legend} ({unit})
      </FieldLegend>
      <BoundInput
        label="Min"
        stored={range.min}
        onChange={(min) => onChange({ ...range, min })}
        toDisplay={toDisplay}
        toStored={toStored}
      />
      <BoundInput
        label="Max"
        stored={range.max}
        onChange={(max) => onChange({ ...range, max })}
        toDisplay={toDisplay}
        toStored={toStored}
      />
    </FieldSet>
  );
}
