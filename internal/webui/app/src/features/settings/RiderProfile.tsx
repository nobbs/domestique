/**
 * The rider's own body and equipment, on their own settings page.
 *
 * The one section here that is neither the browser's preference nor the
 * service's setting: these numbers are this rider's, read and written over
 * their own subject, and every derived training metric downstream needs them.
 *
 * Beside three of the fields sits what the rider's own recent rides suggest.
 * A suggestion is offered, never applied: taking it only fills the field, and
 * nothing uses it until the rider has saved it as their own.
 */

import { IconSparkles, IconUser } from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { FormFooter, FormGroup, FormRow, InfoDot, UnitInput } from "@/components/InsetForm";
import { Panel } from "@/components/PanelHeading";
import { Spinner } from "@/components/ui/spinner";
import { useSetRiderProfile } from "../../api/generated";
import { riderProfileQuery } from "../../api/queries";
import type { RiderParameters, RiderProfile as RiderProfileView } from "../../api/types";
import { Button } from "../../components/Button";
import { Skeleton } from "../../components/ui/skeleton";

/** One editable parameter: what it is called, its unit, and how precisely it reads. */
interface Parameter {
  field: keyof RiderParameters;
  group: "Heart rate" | "Power" | "Rider and bicycle";
  label: string;
  unit: string;
  description: string;
  /**
   * Absent where the parameter has no suggestion to offer. Every suggestion but
   * the stopping habit, which is a distribution the route panel draws rather
   * than a figure this page can put beside a field.
   */
  suggested?: Exclude<keyof RiderProfileView["suggestions"], "stopping">;
}

const PARAMETERS: Parameter[] = [
  {
    field: "maxHeartRateBpm",
    group: "Heart rate",
    label: "Maximum heart rate",
    unit: "bpm",
    description: "The highest rate you reach, which the top of every zone is a share of.",
    suggested: "maxHeartRateBpm",
  },
  {
    field: "restingHeartRateBpm",
    group: "Heart rate",
    label: "Resting heart rate",
    unit: "bpm",
    description:
      "Your rate at rest, which with the maximum gives the reserve that zones are cut from.",
  },
  {
    field: "thresholdHeartRateBpm",
    group: "Heart rate",
    label: "Threshold heart rate",
    unit: "bpm",
    description:
      "The lactate threshold rate, where a zone scheme cuts hard from moderate. Only genuine if measured over a maximal, evenly paced twenty-minute effort.",
    suggested: "thresholdHeartRateBpm",
  },
  {
    field: "functionalThresholdPowerWatts",
    group: "Power",
    label: "Functional threshold power",
    unit: "W",
    description: "The power you hold for an hour, which every ride's load is measured against.",
    suggested: "functionalThresholdPowerWatts",
  },
  {
    field: "riderMassKg",
    group: "Rider and bicycle",
    label: "Rider mass",
    unit: "kg",
    description: "You, dressed to ride.",
  },
  {
    field: "bikeMassKg",
    group: "Rider and bicycle",
    label: "Bike mass",
    unit: "kg",
    description: "The bicycle and everything carried on it.",
  },
  {
    field: "dragAreaM2",
    group: "Rider and bicycle",
    label: "Drag area",
    unit: "m²",
    description:
      "The bicycle's CdA: 0.36 on a road bike's hoods, 0.40 on a gravel bike's hoods, 0.45 sitting up.",
  },
  {
    field: "rollingResistance",
    group: "Rider and bicycle",
    label: "Rolling resistance",
    unit: "",
    description: "The tyres' Crr on tarmac: 0.005 for a road slick, 0.008 for a wide gravel tyre.",
  },
];

const GROUPS = ["Heart rate", "Power", "Rider and bicycle"] as const;

/**
 * The boxes this form has edited, keyed by the parameter each one is: a
 * misspelled key is a compile error rather than an edit that never sends.
 */
type RiderDraft = Partial<Record<keyof RiderParameters, string>>;

/** A stored parameter as its box shows it; an unset one shows an empty box. */
function shown(value: number | undefined): string {
  return value === undefined ? "" : String(value);
}

/**
 * The edit, as the contract takes it. An empty box is left out rather than
 * sent as zero: this write replaces the profile whole, so a box the rider
 * cleared is a parameter cleared.
 */
function submission(profile: RiderParameters, draft: RiderDraft) {
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
    <Panel
      icon={<IconUser size={18} stroke={1.8} />}
      title="Rider profile"
      aside={
        <InfoDot label="Rider profile" framed>
          What this service knows about you, which every derived training figure is worked out from.
          A field left empty is a number this service does not have.
        </InfoDot>
      }
    >
      <div className="grid gap-3">{children}</div>
    </Panel>
  );
}

export function RiderProfile() {
  const id = useId();
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(riderProfileQuery());
  const [draft, setDraft] = useState<RiderDraft>({});
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
        className="grid gap-5"
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          onSave();
        }}
      >
        {GROUPS.map((group) => (
          <FormGroup key={group} title={group}>
            {PARAMETERS.filter((parameter) => parameter.group === group).map((parameter) => {
              const suggestion = parameter.suggested && data.suggestions[parameter.suggested];
              const rounded = suggestion === undefined ? undefined : Math.round(suggestion);
              const inputId = `${id}-${parameter.field}`;

              return (
                <FormRow
                  key={parameter.field}
                  label={parameter.label}
                  srLabel={parameter.unit ? `(${parameter.unit})` : undefined}
                  htmlFor={inputId}
                  hint={
                    <>
                      {parameter.description}
                      {rounded === undefined
                        ? null
                        : ` Your rides of the last 90 days suggest ${rounded} ${parameter.unit}.`}
                    </>
                  }
                >
                  {rounded === undefined ? null : (
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 rounded-full bg-[color-mix(in_oklab,var(--primary)_10%,transparent)] px-2 py-0.5 text-xs hover:bg-[color-mix(in_oklab,var(--primary)_18%,transparent)] focus-visible:outline-2 focus-visible:outline-[var(--accent)] focus-visible:outline-offset-2"
                      aria-label={`Use the suggested ${parameter.label.toLowerCase()}, ${rounded} ${parameter.unit}`}
                      onClick={() =>
                        setDraft((current) => ({ ...current, [parameter.field]: String(rounded) }))
                      }
                    >
                      <IconSparkles size={12} aria-hidden="true" />
                      Use {rounded}
                    </button>
                  )}
                  <UnitInput
                    id={inputId}
                    unit={parameter.unit}
                    type="number"
                    inputMode="decimal"
                    step="any"
                    value={draft[parameter.field] ?? shown(data.profile[parameter.field])}
                    onChange={(event) =>
                      setDraft((current) => ({ ...current, [parameter.field]: event.target.value }))
                    }
                  />
                </FormRow>
              );
            })}
          </FormGroup>
        ))}
        <FormFooter>
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
          <Button
            variant="default"
            aria-label="Save rider profile"
            disabled={save.isPending}
            onClick={onSave}
          >
            {save.isPending ? <Spinner aria-label="Saving" /> : null}
            Save
          </Button>
        </FormFooter>
      </form>
    </CardShell>
  );
}
