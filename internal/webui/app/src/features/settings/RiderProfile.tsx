/**
 * The rider's own body and equipment, on their own settings page.
 *
 * The one section here that is neither the browser's preference nor the
 * service's setting: these numbers are this rider's, read and written over
 * their own subject, and every derived training metric downstream needs them.
 *
 * Beside two of the fields sits what the rider's own recent rides suggest.
 * A suggestion is offered, never applied: nothing uses one until the rider has
 * typed it in and saved it as their own.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { useSetRiderProfile } from "../../api/generated";
import { riderProfileQuery } from "../../api/queries";
import type { RiderParameters, RiderProfile as RiderProfileView } from "../../api/types";
import { Button } from "../../components/Button";
import { Skeleton } from "../../components/ui/skeleton";

/** One editable parameter: what it is called, its unit, and how precisely it reads. */
interface Parameter {
  field: keyof RiderParameters;
  label: string;
  unit: string;
  description: string;
  /** Absent where the parameter has no suggestion to offer. */
  suggested?: keyof RiderProfileView["suggestions"];
}

const PARAMETERS: Parameter[] = [
  {
    field: "maxHeartRateBpm",
    label: "Maximum heart rate",
    unit: "bpm",
    description: "The highest rate you reach, which the top of every zone is a share of.",
    suggested: "maxHeartRateBpm",
  },
  {
    field: "restingHeartRateBpm",
    label: "Resting heart rate",
    unit: "bpm",
    description:
      "Your rate at rest, which with the maximum gives the reserve that zones are cut from.",
  },
  {
    field: "thresholdHeartRateBpm",
    label: "Threshold heart rate",
    unit: "bpm",
    description: "The lactate threshold rate, where a zone scheme cuts hard from moderate.",
  },
  {
    field: "functionalThresholdPowerWatts",
    label: "Functional threshold power",
    unit: "W",
    description: "The power you hold for an hour, which every ride's load is measured against.",
    suggested: "functionalThresholdPowerWatts",
  },
  {
    field: "riderMassKg",
    label: "Rider mass",
    unit: "kg",
    description: "You, dressed to ride.",
  },
  {
    field: "bikeMassKg",
    label: "Bike mass",
    unit: "kg",
    description: "The bicycle and everything carried on it.",
  },
];

/** A stored parameter as its box shows it; an unset one shows an empty box. */
function shown(value: number | undefined): string {
  return value === undefined ? "" : String(value);
}

/**
 * The edit, as the contract takes it. An empty box is left out rather than
 * sent as zero: this write replaces the profile whole, so a box the rider
 * cleared is a parameter cleared.
 */
function submission(profile: RiderParameters, draft: Partial<Record<string, string>>) {
  const edited: RiderParameters = {};
  for (const { field } of PARAMETERS) {
    const value = draft[field] ?? shown(profile[field]);
    const parsed = Number(value.trim());
    if (value.trim() !== "" && Number.isFinite(parsed)) {
      edited[field] = parsed;
    }
  }

  return edited;
}

function CardShell({ children }: { children: React.ReactNode }) {
  return (
    <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
      <CardHeader>
        <CardTitle role="heading" aria-level={2}>
          Rider profile
        </CardTitle>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

export function RiderProfile() {
  const id = useId();
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(riderProfileQuery());
  const [draft, setDraft] = useState<Partial<Record<string, string>>>({});
  const save = useSetRiderProfile({
    mutation: {
      onSuccess: async () => {
        setDraft({});
        await queryClient.invalidateQueries({ queryKey: riderProfileQuery().queryKey });
      },
    },
  });

  if (isPending) {
    return (
      <CardShell>
        <Skeleton className="h-64 w-full" role="status" aria-label="Loading your rider profile" />
      </CardShell>
    );
  }
  if (isError) {
    return (
      <CardShell>
        <p className="text-sm text-[var(--alert)]" role="alert">
          The service did not say what your profile holds.
        </p>
      </CardShell>
    );
  }

  const edited = Object.keys(draft).length > 0;
  const onSave = () => save.mutate({ data: submission(data.profile, draft) });

  return (
    <CardShell>
      <form
        className="grid gap-6"
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          onSave();
        }}
      >
        <FieldDescription>
          What this service knows about you, which every derived training figure is worked out from.
          A field left empty is a number this service does not have.
        </FieldDescription>
        <FieldGroup>
          {PARAMETERS.map((parameter) => {
            const suggestion = parameter.suggested && data.suggestions[parameter.suggested];

            return (
              <Field key={parameter.field}>
                <FieldLabel htmlFor={`${id}-${parameter.field}`}>
                  {parameter.label} ({parameter.unit})
                </FieldLabel>
                <Input
                  id={`${id}-${parameter.field}`}
                  type="number"
                  inputMode="decimal"
                  step="any"
                  value={draft[parameter.field] ?? shown(data.profile[parameter.field])}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, [parameter.field]: event.target.value }))
                  }
                />
                <FieldDescription>
                  {parameter.description}
                  {suggestion === undefined ? null : (
                    <>
                      {" "}
                      Your rides of the last 90 days suggest {Math.round(suggestion)}{" "}
                      {parameter.unit}.
                    </>
                  )}
                </FieldDescription>
              </Field>
            );
          })}
        </FieldGroup>
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="default"
            aria-label="Save rider profile"
            disabled={save.isPending}
            onClick={onSave}
          >
            {save.isPending ? <Spinner aria-label="Saving" /> : null}
            Save
          </Button>
          {save.isError ? (
            <p className="text-sm text-[var(--alert)]" role="alert">
              {save.error instanceof Error && save.error.message
                ? save.error.message
                : "Your profile was not saved."}
            </p>
          ) : null}
          {save.isSuccess && !edited ? (
            <p className="text-sm text-[var(--ink-2)]" aria-live="polite">
              Saved.
            </p>
          ) : null}
        </div>
      </form>
    </CardShell>
  );
}
