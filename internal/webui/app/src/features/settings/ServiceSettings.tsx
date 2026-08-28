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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
  SOURCE_PROVIDERS,
  type SourceProvider,
  type SourceSettings,
  SUCCESS_POLICIES,
  type SuccessPolicy,
} from "../../api/types";
import { Button } from "../../components/Button";

const SECONDS_PER_HOUR = 3600;
const SECONDS_PER_MINUTE = 60;

const POLICY_LABELS: Record<SuccessPolicy, string> = {
  every: "One message per successful run",
  quiet: "Nothing — leaving failures and recoveries as the only traffic",
  digest: "One summary per period",
};

const PROVIDER_LABELS: Record<SourceProvider, string> = {
  veloplanner: "VeloPlanner",
  komoot: "Komoot",
};

/**
 * Where a library is reached when it is first turned on, which is the address
 * each provider publishes. It is editable afterwards, so this is a starting
 * point rather than a rule.
 */
const PROVIDER_BASE_URLS: Record<SourceProvider, string> = {
  veloplanner: "https://veloplanner.com",
  komoot: "https://api.komoot.de",
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

function inMinutes(seconds: number): number {
  return seconds / SECONDS_PER_MINUTE;
}

function fromMinutes(minutes: number): number {
  return Math.round(minutes * SECONDS_PER_MINUTE);
}

/**
 * The lines of a list typed one per line. They are kept as typed while they are
 * being typed, so the newline that starts an entry survives; the blanks are
 * dropped on the way out.
 */
function entered(lines: string[]): string[] {
  return lines.map((line) => line.trim()).filter(Boolean);
}

/**
 * An entry with no dark style omits the field rather than carrying an empty
 * one: the contract types it as a URL, and "unset" is not one.
 */
function withDarkStyle(basemap: BrowserBasemap, styleUrlDark: string): BrowserBasemap {
  const { styleUrlDark: _cleared, ...rest } = basemap;

  return styleUrlDark ? { ...rest, styleUrlDark } : rest;
}

/**
 * One credential. The service never says what it holds, only whether it holds
 * one, so this offers a replacement rather than a value: an empty box leaves
 * the stored credential exactly as it is.
 */
function SecretField({
  id,
  label,
  isSet,
  value,
  onChange,
}: {
  id: string;
  label: string;
  isSet: boolean;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        type="password"
        autoComplete="off"
        value={value}
        placeholder={isSet ? "Stored — type to replace" : "Not set"}
        onChange={(event) => onChange(event.target.value)}
      />
      <FieldDescription>{isSet ? "Stored." : "Not set."}</FieldDescription>
    </Field>
  );
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
  // Only the credentials typed into this form, keyed the way the service stores
  // them. What it already holds is never in here: it is never sent to the page.
  const [secrets, setSecrets] = useState<Record<string, string>>({});
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
        setSecrets({});
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
  const secretOf = (name: string) => secrets[name] ?? "";
  const editSecret = (name: string, value: string) => setSecrets({ ...secrets, [name]: value });

  const sourceOf = (provider: SourceProvider) =>
    values.sources.find((source) => source.provider === provider);
  // Rebuilt from the whole list rather than spliced, so the libraries stay in
  // the order this page offers them however they are turned on and off.
  const editSource = (provider: SourceProvider, next: SourceSettings | undefined) =>
    edit({
      sources: SOURCE_PROVIDERS.map((each) => (each === provider ? next : sourceOf(each))).filter(
        (source): source is SourceSettings => source !== undefined,
      ),
    });

  const submit = () => {
    // Only the credentials that were typed into are sent: the rest are left
    // exactly as they are, which is the whole reason this form can be shown
    // without ever having been told them.
    const replaced = Object.fromEntries(
      Object.entries(secrets).filter(([, value]) => value !== ""),
    );
    save.mutate({
      data: {
        sync: values.sync,
        notifications: values.notifications,
        basemaps: values.basemaps,
        surface: { ...values.surface, regions: entered(values.surface.regions) },
        wahoo: { ...values.wahoo, targets: entered(values.wahoo.targets) },
        sources: values.sources,
        rideModel: values.rideModel,
        ...(Object.keys(replaced).length > 0 ? { secrets: replaced } : {}),
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
      {/*
       * What the service is waiting for, said where it is filled in. Until this
       * is empty the schedule runs and does nothing, which is a state an
       * operator should read here rather than infer from a run that did.
       */}
      {values.missing.length > 0 ? (
        <Alert
          variant="destructive"
          className="border-[var(--rule)] bg-[var(--panel)] p-4"
          aria-labelledby={`${id}-missing`}
        >
          <AlertTitle id={`${id}-missing`}>Not finished configuring</AlertTitle>
          <AlertDescription>These are still needed: {values.missing.join(", ")}.</AlertDescription>
        </Alert>
      ) : null}

      <FieldSet>
        <FieldLegend>Wahoo</FieldLegend>
        <FieldDescription>
          The registered application this service writes routes with, and the accounts it writes
          them to.
        </FieldDescription>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor={`${id}-wahoo-api`}>API address</FieldLabel>
            <Input
              id={`${id}-wahoo-api`}
              type="url"
              value={values.wahoo.apiBaseUrl}
              onChange={(event) =>
                edit({ wahoo: { ...values.wahoo, apiBaseUrl: event.target.value } })
              }
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-wahoo-oauth`}>Authorization address</FieldLabel>
            <Input
              id={`${id}-wahoo-oauth`}
              type="url"
              value={values.wahoo.oauthBaseUrl}
              onChange={(event) =>
                edit({ wahoo: { ...values.wahoo, oauthBaseUrl: event.target.value } })
              }
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-wahoo-client`}>Client ID</FieldLabel>
            <Input
              id={`${id}-wahoo-client`}
              value={values.wahoo.clientId}
              onChange={(event) =>
                edit({ wahoo: { ...values.wahoo, clientId: event.target.value } })
              }
            />
          </Field>
          <SecretField
            id={`${id}-wahoo-secret`}
            label="Client secret"
            isSet={values.secretsSet["wahoo.client_secret"] ?? false}
            value={secretOf("wahoo.client_secret")}
            onChange={(value) => editSecret("wahoo.client_secret", value)}
          />
          <Field>
            <FieldLabel htmlFor={`${id}-targets`}>Accounts, one per line</FieldLabel>
            <Textarea
              id={`${id}-targets`}
              rows={2}
              value={values.wahoo.targets.join("\n")}
              onChange={(event) =>
                edit({ wahoo: { ...values.wahoo, targets: event.target.value.split("\n") } })
              }
            />
            <FieldDescription>
              A name here is the identity every authorization, route and recorded run is stored
              under, so renaming one leaves that account's history behind rather than moving it. Two
              at most.
            </FieldDescription>
          </Field>
        </FieldGroup>
      </FieldSet>

      <FieldSet>
        <FieldLegend>Libraries</FieldLegend>
        <FieldDescription>
          Where the routes come from. A library is read with an account of its own, and the address
          is both what is read and what a stage is linked back to.
        </FieldDescription>
        <FieldGroup>
          {SOURCE_PROVIDERS.map((provider) => {
            const source = sourceOf(provider);

            return (
              <div key={provider} className="grid gap-3 rounded-lg border border-[var(--rule)] p-3">
                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldTitle>{PROVIDER_LABELS[provider]}</FieldTitle>
                  </FieldContent>
                  <Switch
                    checked={source !== undefined}
                    aria-label={`Read ${PROVIDER_LABELS[provider]}`}
                    onCheckedChange={(read) =>
                      editSource(
                        provider,
                        read ? { provider, baseUrl: PROVIDER_BASE_URLS[provider] } : undefined,
                      )
                    }
                  />
                </Field>
                {source ? (
                  <>
                    <Field>
                      <FieldLabel htmlFor={`${id}-${provider}-url`}>Address</FieldLabel>
                      <Input
                        id={`${id}-${provider}-url`}
                        type="url"
                        value={source.baseUrl}
                        onChange={(event) =>
                          editSource(provider, { ...source, baseUrl: event.target.value })
                        }
                      />
                    </Field>
                    <SecretField
                      id={`${id}-${provider}-email`}
                      label="Email"
                      isSet={values.secretsSet[`${provider}.email`] ?? false}
                      value={secretOf(`${provider}.email`)}
                      onChange={(value) => editSecret(`${provider}.email`, value)}
                    />
                    <SecretField
                      id={`${id}-${provider}-password`}
                      label="Password"
                      isSet={values.secretsSet[`${provider}.password`] ?? false}
                      value={secretOf(`${provider}.password`)}
                      onChange={(value) => editSecret(`${provider}.password`, value)}
                    />
                  </>
                ) : null}
              </div>
            );
          })}
        </FieldGroup>
      </FieldSet>

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
              step="any"
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
          <Field>
            <FieldLabel htmlFor={`${id}-initial-delay`}>
              Wait before the first run (minutes)
            </FieldLabel>
            <Input
              id={`${id}-initial-delay`}
              type="number"
              min={1}
              step="any"
              value={inMinutes(values.sync.initialDelaySeconds)}
              onChange={(event) =>
                edit({
                  sync: {
                    ...values.sync,
                    initialDelaySeconds: fromMinutes(Number(event.target.value)),
                  },
                })
              }
            />
            <FieldDescription>
              Read by the start it delays, so this one takes effect on the next restart rather than
              the next run.
            </FieldDescription>
          </Field>
        </FieldGroup>
      </FieldSet>

      <FieldSet>
        <FieldLegend>Ride model</FieldLegend>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor={`${id}-coefficients`}>Coefficient file</FieldLabel>
            <Input
              id={`${id}-coefficients`}
              value={values.rideModel.coefficientsFile}
              onChange={(event) => edit({ rideModel: { coefficientsFile: event.target.value } })}
            />
            <FieldDescription>
              An absolute path, on the machine the service runs on, to the file the fitting tooling
              produced. Empty leaves stages without a predicted moving time.
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
              step="any"
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
          <SecretField
            id={`${id}-pushover-token`}
            label="Pushover application token"
            isSet={values.secretsSet["notifications.pushover.application_token"] ?? false}
            value={secretOf("notifications.pushover.application_token")}
            onChange={(value) => editSecret("notifications.pushover.application_token", value)}
          />
          <SecretField
            id={`${id}-pushover-user`}
            label="Pushover user key"
            isSet={values.secretsSet["notifications.pushover.user_key"] ?? false}
            value={secretOf("notifications.pushover.user_key")}
            onChange={(value) => editSecret("notifications.pushover.user_key", value)}
          />
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
              step="any"
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
