/**
 * The settings the service holds, on the page that already holds this
 * browser's own preferences.
 *
 * The two are next to each other and are not the same kind of thing, which is
 * why they are two cards. Above, a choice this browser remembers for the reader
 * sitting at it; here, a change to how the service behaves for everyone it
 * syncs, stored in its database and in force from the next run or the next
 * request onwards.
 *
 * The whole object is sent on every save. The form holds every value, and the
 * endpoint takes the object rather than a patch, so what is sent is what is
 * shown — a field left out would be a field this page never read.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, type ReactNode, useId, useRef, useState } from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useSetSettings } from "../../api/generated";
import { settingsQuery, webUIConfigQuery } from "../../api/queries";
import {
  type BrowserBasemap,
  type Settings,
  SUCCESS_POLICIES,
  type SuccessPolicy,
} from "../../api/types";
import { Button } from "../../components/Button";

const SECONDS_PER_HOUR = 3600;

const POLICY_LABELS: Record<SuccessPolicy, string> = {
  every: "One message per successful run",
  quiet: "Nothing — leaving failures and recoveries as the only traffic",
  digest: "One summary per period",
};

/**
 * Durations cross the wire in seconds and are read here in hours, which is the
 * unit every one of these is actually set in. A value that is not a whole
 * number of hours shows as a fraction rather than being rounded to one.
 */
function inHours(seconds: number): number {
  return seconds / SECONDS_PER_HOUR;
}

function fromHours(hours: number): number {
  return Math.round(hours * SECONDS_PER_HOUR);
}

/**
 * An entry with no dark style omits the field rather than carrying an empty
 * one: the contract types it as a URL, and "unset" is not one.
 */
function withDarkStyle(basemap: BrowserBasemap, styleUrlDark: string): BrowserBasemap {
  const { styleUrlDark: _cleared, ...rest } = basemap;

  return styleUrlDark ? { ...rest, styleUrlDark } : rest;
}

/** The card chrome, shared by the form and by the two states before it. */
function SettingsCard({ children }: { children: ReactNode }) {
  return (
    <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
      <CardHeader>
        <CardTitle role="heading" aria-level={2}>
          Service settings
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-8">{children}</CardContent>
    </Card>
  );
}

export function ServiceSettings() {
  const { data, isPending, isError } = useQuery(settingsQuery());

  if (isPending) {
    return (
      <SettingsCard>
        <Skeleton className="h-64 w-full" role="status" aria-label="Loading service settings" />
      </SettingsCard>
    );
  }
  if (isError) {
    return (
      <SettingsCard>
        <p className="text-sm text-[var(--alert)]" role="alert">
          The service did not say what it is set to.
        </p>
      </SettingsCard>
    );
  }

  return (
    <SettingsCard>
      <SettingsForm settings={data} />
    </SettingsCard>
  );
}

function SettingsForm({ settings }: { settings: Settings }) {
  const queryClient = useQueryClient();
  const id = useId();
  // Null until the reader touches something, so an answer that arrives while
  // the form is untouched is simply what the form shows.
  const [draft, setDraft] = useState<Settings | null>(null);
  const [confirmingDeletion, setConfirmingDeletion] = useState(false);
  // A basemap row has no identity in the settings — two rows are both blank
  // while they are being typed — so one is kept beside them, or removing the
  // first row would move every value below it up into a different input.
  const [rowKeys, setRowKeys] = useState(() => settings.basemaps.map((_, index) => index));
  const nextRowKey = useRef(settings.basemaps.length);

  const save = useSetSettings({
    mutation: {
      onSuccess: async () => {
        setDraft(null);
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: settingsQuery().queryKey }),
          // The basemap list is served to the page from here as well, and the
          // map picker reads that copy.
          queryClient.invalidateQueries({ queryKey: webUIConfigQuery().queryKey }),
        ]);
      },
    },
  });

  const values = draft ?? settings;
  const edit = (change: Partial<Settings>) => setDraft({ ...values, ...change });
  const replaceBasemap = (index: number, next: BrowserBasemap) =>
    edit({ basemaps: values.basemaps.map((basemap, at) => (at === index ? next : basemap)) });

  const submit = () => {
    save.mutate({
      data: {
        ...values,
        // The lines the reader typed are kept as typed while they type, so the
        // newline that starts a region survives; the blanks are dropped here.
        surface: {
          ...values.surface,
          regions: values.surface.regions.map((region) => region.trim()).filter(Boolean),
        },
      },
    });
  };

  return (
    <form
      className="grid gap-8"
      onSubmit={(event: FormEvent) => {
        event.preventDefault();
        submit();
      }}
    >
      <FieldSet>
        <FieldLegend>Synchronisation</FieldLegend>
        <FieldGroup>
          <Field orientation="horizontal">
            <FieldContent>
              <FieldTitle>Let an empty library delete a target's routes</FieldTitle>
              <FieldDescription>
                A read that finds nothing at the source is otherwise treated as a fault and the
                write is held. This stays on until you turn it off again — it does not reset after
                one run.
              </FieldDescription>
            </FieldContent>
            <Switch
              checked={values.sync.allowEmptySourceDeletion}
              aria-label="Let an empty library delete a target's routes"
              onCheckedChange={(next) =>
                next
                  ? setConfirmingDeletion(true)
                  : edit({ sync: { ...values.sync, allowEmptySourceDeletion: false } })
              }
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-stale`}>Call the library stale after (hours)</FieldLabel>
            <Input
              id={`${id}-stale`}
              type="number"
              min={1}
              value={inHours(values.sync.staleAfterSeconds)}
              onChange={(event) =>
                edit({
                  sync: {
                    ...values.sync,
                    staleAfterSeconds: fromHours(Number(event.target.value)),
                  },
                })
              }
            />
            <FieldDescription>
              How long the last successful read may stand before the status page reports the
              inventory as stale, and says so.
            </FieldDescription>
          </Field>
        </FieldGroup>
      </FieldSet>

      <FieldSet>
        <FieldLegend>Notifications</FieldLegend>
        <FieldGroup>
          <Field orientation="horizontal">
            <FieldContent>
              <FieldTitle>Send notifications</FieldTitle>
              <FieldDescription>
                Off silences the whole channel, not only the routine ones: while it is off a failed
                run and a stale library go unsent as surely as a success does.
              </FieldDescription>
            </FieldContent>
            <Switch
              checked={values.notifications.enabled}
              aria-label="Send notifications"
              onCheckedChange={(enabled) =>
                edit({ notifications: { ...values.notifications, enabled } })
              }
            />
          </Field>
          <FieldSet>
            <FieldLegend>When a run succeeds, send</FieldLegend>
            <RadioGroup
              value={values.notifications.successPolicy}
              onValueChange={(policy) =>
                edit({
                  notifications: {
                    ...values.notifications,
                    successPolicy: policy as SuccessPolicy,
                  },
                })
              }
            >
              <FieldGroup>
                {SUCCESS_POLICIES.map((policy) => (
                  <FieldLabel key={policy}>
                    <Field orientation="horizontal">
                      <RadioGroupItem value={policy} />
                      <span>{POLICY_LABELS[policy]}</span>
                    </Field>
                  </FieldLabel>
                ))}
              </FieldGroup>
            </RadioGroup>
          </FieldSet>
          <Field>
            <FieldLabel htmlFor={`${id}-digest`}>One summary covers (hours)</FieldLabel>
            <Input
              id={`${id}-digest`}
              type="number"
              min={1}
              value={inHours(values.notifications.digestIntervalSeconds)}
              onChange={(event) =>
                edit({
                  notifications: {
                    ...values.notifications,
                    digestIntervalSeconds: fromHours(Number(event.target.value)),
                  },
                })
              }
            />
            <FieldDescription>Read by the summary policy alone.</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-pushover`}>Pushover address</FieldLabel>
            <Input
              id={`${id}-pushover`}
              type="url"
              value={values.notifications.pushoverBaseUrl}
              onChange={(event) =>
                edit({
                  notifications: { ...values.notifications, pushoverBaseUrl: event.target.value },
                })
              }
            />
            <FieldDescription>
              The origin the application token and user key are sent to.
            </FieldDescription>
          </Field>
        </FieldGroup>
      </FieldSet>

      <FieldSet>
        <FieldLegend>Basemaps</FieldLegend>
        <FieldDescription>
          The cartography this page offers. An entry with a dark style switches between the two with
          the system colour scheme; an entry whose own ground is dark whatever the scheme is —
          imagery — says so instead, and the two cannot both be set.
        </FieldDescription>
        <FieldGroup>
          {values.basemaps.map((basemap, index) => (
            <div
              key={rowKeys[index]}
              className="grid gap-3 rounded-lg border border-[var(--rule)] p-3"
            >
              <Field>
                <FieldLabel htmlFor={`${id}-name-${rowKeys[index]}`}>Name</FieldLabel>
                <Input
                  id={`${id}-name-${rowKeys[index]}`}
                  value={basemap.name}
                  onChange={(event) =>
                    replaceBasemap(index, { ...basemap, name: event.target.value })
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${id}-style-${rowKeys[index]}`}>Style URL</FieldLabel>
                <Input
                  id={`${id}-style-${rowKeys[index]}`}
                  type="url"
                  value={basemap.styleUrl}
                  onChange={(event) =>
                    replaceBasemap(index, { ...basemap, styleUrl: event.target.value })
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${id}-dark-${rowKeys[index]}`}>
                  Dark style URL (optional)
                </FieldLabel>
                <Input
                  id={`${id}-dark-${rowKeys[index]}`}
                  type="url"
                  value={basemap.styleUrlDark ?? ""}
                  onChange={(event) =>
                    replaceBasemap(index, withDarkStyle(basemap, event.target.value))
                  }
                />
              </Field>
              <Field orientation="horizontal">
                <FieldContent>
                  <FieldTitle>This style is dark cartography</FieldTitle>
                </FieldContent>
                <Switch
                  checked={basemap.darkCartography ?? false}
                  disabled={Boolean(basemap.styleUrlDark)}
                  aria-label={`This style is dark cartography: basemap ${index + 1}`}
                  onCheckedChange={(darkCartography) =>
                    replaceBasemap(index, { ...basemap, darkCartography })
                  }
                />
              </Field>
              <div>
                <Button
                  variant="danger"
                  disabled={values.basemaps.length === 1}
                  aria-label={`Remove basemap ${index + 1}`}
                  onClick={() => {
                    edit({ basemaps: values.basemaps.filter((_, at) => at !== index) });
                    setRowKeys(rowKeys.filter((_, at) => at !== index));
                  }}
                >
                  Remove
                </Button>
              </div>
            </div>
          ))}
          <div>
            <Button
              variant="standard"
              onClick={() => {
                edit({ basemaps: [...values.basemaps, { name: "", styleUrl: "" }] });
                setRowKeys([...rowKeys, nextRowKey.current]);
                nextRowKey.current += 1;
              }}
            >
              Add a basemap
            </Button>
          </div>
        </FieldGroup>
      </FieldSet>

      <FieldSet>
        <FieldLegend>Surface classification</FieldLegend>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor={`${id}-regions`}>Geofabrik regions, one per line</FieldLabel>
            <Textarea
              id={`${id}-regions`}
              rows={3}
              value={values.surface.regions.join("\n")}
              onChange={(event) =>
                edit({ surface: { ...values.surface, regions: event.target.value.split("\n") } })
              }
            />
            <FieldDescription>
              An empty list switches classification off. Naming a region does not build the index:
              the next rebuild on the schedule below does, and stages are classified on the pass
              after that.
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-rebuild`}>Rebuild the index every (hours)</FieldLabel>
            <Input
              id={`${id}-rebuild`}
              type="number"
              min={1}
              value={inHours(values.surface.rebuildIntervalSeconds)}
              onChange={(event) =>
                edit({
                  surface: {
                    ...values.surface,
                    rebuildIntervalSeconds: fromHours(Number(event.target.value)),
                  },
                })
              }
            />
            <FieldDescription>
              Required whether or not a region is named: the schedule runs either way, and with no
              region it builds nothing.
            </FieldDescription>
          </Field>
        </FieldGroup>
      </FieldSet>

      <div className="flex flex-wrap items-center gap-3">
        <Button variant="primary" disabled={save.isPending} onClick={submit}>
          {save.isPending ? <Spinner aria-label="Saving" /> : null}
          Save settings
        </Button>
        {/*
         * Announced rather than waited for, as elsewhere: the reader has just
         * pressed something, and the service's own words are what says which
         * value it refused.
         */}
        {save.isError ? (
          <p className="text-sm text-[var(--alert)]" role="alert">
            {save.error instanceof Error && save.error.message
              ? save.error.message
              : "Those settings were not saved."}
          </p>
        ) : null}
        {save.isSuccess && !draft ? (
          <p className="text-sm text-[var(--ink-2)]" aria-live="polite">
            Saved. It is in force from the next run or the next request.
          </p>
        ) : null}
      </div>

      {/*
       * The one switch on this page that asks first. It is the one that lets a
       * synchronisation delete an entire library, and it is a switch rather than
       * a run, so the confirmation is about what it will keep permitting.
       */}
      <AlertDialog open={confirmingDeletion} onOpenChange={setConfirmingDeletion}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Let an empty library delete a target's routes?</AlertDialogTitle>
            <AlertDialogDescription>
              While this is on, a read that finds no routes at the source is taken at its word, and
              the next write removes the routes Domestique put on every target. It stays on until
              you turn it off again.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="danger"
              onClick={() => edit({ sync: { ...values.sync, allowEmptySourceDeletion: true } })}
            >
              Allow it
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </form>
  );
}
