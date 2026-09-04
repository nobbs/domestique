/**
 * The settings the service holds, on the page that already holds this
 * browser's own preferences.
 *
 * The two are next to each other and are not the same kind of thing, which is
 * why the browser's are one card and these are their own. Above, a choice this
 * browser remembers for the reader sitting at it; here, a change to how the
 * service behaves for everyone it syncs, stored in its database and in force
 * from the next run or the next request onwards.
 *
 * A section is a card with its own save. Each one is sent whole to the endpoint
 * that owns it — the form holds every field of it, and the endpoint takes the
 * object rather than a patch — so a save carries what its own card holds and
 * touches nothing else. An edit left unsaved in one card is therefore still
 * there after another card is saved, and is never written by it.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, type ReactNode, useId, useRef, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import {
  useSetAlerts,
  useSetBasemaps,
  useSetNotifications,
  useSetRideModel,
  useSetSource,
  useSetSurface,
  useSetSync,
  useSetTimezone,
  useSetWahooApplication,
} from "../../api/generated";
import { settingsQuery, webUIConfigQuery } from "../../api/queries";
import {
  type AlertSetting,
  type BrowserBasemap,
  type Settings,
  SOURCE_PROVIDERS,
  type SourceProvider,
} from "../../api/types";
import { Button } from "../../components/Button";
import { BasemapStrip } from "../../components/map/BasemapPreview";
import { providerLabel } from "../../lib/provider";
import { RegionPicker } from "../settings/regions/RegionPicker";

const SECONDS_PER_HOUR = 3600;
const SECONDS_PER_MINUTE = 60;

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
 * An entry with no dark style omits the field rather than carrying an empty
 * one: the contract types it as a URL, and "unset" is not one.
 */
function withDarkStyle(basemap: BrowserBasemap, styleUrlDark: string): BrowserBasemap {
  const { styleUrlDark: _cleared, ...rest } = basemap;

  return styleUrlDark ? { ...rest, styleUrlDark } : rest;
}

/**
 * A credential this card was typed into, ready to send. An untouched box is
 * absent rather than empty: the service leaves a credential it is not sent
 * exactly as it is, which is what lets this page offer a replacement for one it
 * was never told.
 */
function replacement<Field extends string>(
  field: Field,
  value: string,
): Partial<Record<Field, string>> {
  return value ? ({ [field]: value } as Record<Field, string>) : {};
}

/** One credential, offered for replacement rather than shown. */
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

/**
 * What a section reads off its own save, which is all of a mutation it shows.
 */
interface SaveState {
  isPending: boolean;
  isError: boolean;
  isSuccess: boolean;
  error: unknown;
}

/**
 * Every save invalidates the same two reads: the settings themselves, and the
 * page configuration the map picker takes its basemap list from.
 */
function useSettingsInvalidation(): () => Promise<unknown> {
  const queryClient = useQueryClient();

  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: settingsQuery().queryKey }),
      queryClient.invalidateQueries({ queryKey: webUIConfigQuery().queryKey }),
    ]);
}

/**
 * What every section hands its mutation: forget the edit, then read back what
 * the service now holds.
 */
function saving(reset: () => void, invalidate: () => Promise<unknown>) {
  return {
    mutation: {
      onSuccess: async () => {
        reset();
        await invalidate();
      },
    },
  };
}

/**
 * One section of the settings: its own card, its own fields, and its own save.
 *
 * The button says only "Save", and is named for its section to whoever cannot
 * see which card it sits in.
 */
function Section({
  title,
  description,
  save,
  onSave,
  edited,
  children,
}: {
  title: string;
  description?: ReactNode;
  save: SaveState;
  onSave: () => void;
  edited: boolean;
  children: ReactNode;
}) {
  return (
    <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
      <CardHeader>
        <CardTitle role="heading" aria-level={3}>
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-6"
          onSubmit={(event: FormEvent) => {
            event.preventDefault();
            onSave();
          }}
        >
          {description ? <FieldDescription>{description}</FieldDescription> : null}
          <FieldGroup>{children}</FieldGroup>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="default"
              aria-label={`Save ${title}`}
              disabled={save.isPending}
              onClick={onSave}
            >
              {save.isPending ? <Spinner aria-label="Saving" /> : null}
              Save
            </Button>
            {/*
             * Announced rather than waited for, as elsewhere: the reader has
             * just pressed something, and the service's own words are what says
             * which value it refused.
             */}
            {save.isError ? (
              <p className="text-sm text-[var(--alert)]" role="alert">
                {save.error instanceof Error && save.error.message
                  ? save.error.message
                  : "Those settings were not saved."}
              </p>
            ) : null}
            {save.isSuccess && !edited ? (
              <p className="text-sm text-[var(--ink-2)]" aria-live="polite">
                Saved. It is in force from the next run or the next request.
              </p>
            ) : null}
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

/** The card chrome for the two states before there are any settings to show. */
function SettingsCard({ children }: { children: ReactNode }) {
  return (
    <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
      <CardHeader>
        <CardTitle role="heading" aria-level={3}>
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
      <Sections>
        <SettingsCard>
          <Skeleton className="h-64 w-full" role="status" aria-label="Loading service settings" />
        </SettingsCard>
      </Sections>
    );
  }
  if (isError) {
    return (
      <Sections>
        <SettingsCard>
          <p className="text-sm text-[var(--alert)]" role="alert">
            The service did not say what it is set to.
          </p>
        </SettingsCard>
      </Sections>
    );
  }

  return (
    <Sections>
      <Missing missing={data.missing} />
      <Timezone settings={data} />
      <WahooApplication settings={data} />
      {SOURCE_PROVIDERS.map((provider) => (
        <SourceSettingsSection key={provider} provider={provider} settings={data} />
      ))}
      <Notifications settings={data} />
      <Alerts settings={data} />
      <Basemaps settings={data} />
      <SurfaceClassification settings={data} />
      <RideModel settings={data} />
      <Sync settings={data} />
    </Sections>
  );
}

/** The heading these cards sit under, and the space between them. */
function Sections({ children }: { children: ReactNode }) {
  return (
    <section className="grid gap-6" aria-labelledby="service-settings">
      <h2 id="service-settings" className="text-2xl font-semibold tracking-tight">
        Service settings
      </h2>
      {children}
    </section>
  );
}

/**
 * What the service is waiting for, said above the cards it is filled into.
 * Until this is empty the schedule runs and does nothing, which is a state an
 * operator should read here rather than infer from a run that did.
 */
function Missing({ missing }: { missing: string[] }) {
  const id = useId();

  if (missing.length === 0) {
    return null;
  }

  return (
    <Alert
      variant="destructive"
      className="border-[var(--rule)] bg-[var(--panel)] p-4"
      aria-labelledby={id}
    >
      <AlertTitle id={id}>Not finished configuring</AlertTitle>
      <AlertDescription>These are still needed: {missing.join(", ")}.</AlertDescription>
    </Alert>
  );
}

function WahooApplication({ settings }: { settings: Settings }) {
  const id = useId();
  const invalidate = useSettingsInvalidation();
  const [draft, setDraft] = useState<Settings["wahoo"] | null>(null);
  const [secret, setSecret] = useState("");
  const save = useSetWahooApplication(
    saving(() => {
      setDraft(null);
      setSecret("");
    }, invalidate),
  );

  const values = draft ?? settings.wahoo;
  const edit = (change: Partial<Settings["wahoo"]>) => setDraft({ ...values, ...change });

  return (
    <Section
      title="Wahoo application"
      description="The registered application this service writes routes with."
      save={save}
      edited={draft !== null}
      onSave={() =>
        save.mutate({
          data: {
            apiBaseUrl: values.apiBaseUrl,
            oauthBaseUrl: values.oauthBaseUrl,
            clientId: values.clientId,
            ...replacement("clientSecret", secret),
          },
        })
      }
    >
      <Field>
        <FieldLabel htmlFor={`${id}-api`}>API address</FieldLabel>
        <Input
          id={`${id}-api`}
          type="url"
          value={values.apiBaseUrl}
          onChange={(event) => edit({ apiBaseUrl: event.target.value })}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-oauth`}>Authorization address</FieldLabel>
        <Input
          id={`${id}-oauth`}
          type="url"
          value={values.oauthBaseUrl}
          onChange={(event) => edit({ oauthBaseUrl: event.target.value })}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-client`}>Client ID</FieldLabel>
        <Input
          id={`${id}-client`}
          value={values.clientId}
          onChange={(event) => edit({ clientId: event.target.value })}
        />
      </Field>
      <SecretField
        id={`${id}-secret`}
        label="Client secret"
        isSet={settings.secretsSet["wahoo.client_secret"] ?? false}
        value={secret}
        onChange={setSecret}
      />
    </Section>
  );
}

/**
 * One source, with the account it is read with. Turning it off stops it being
 * read and leaves the account stored, so turning it back on does not ask for
 * the credentials again.
 */
function SourceSettingsSection({
  provider,
  settings,
}: {
  provider: SourceProvider;
  settings: Settings;
}) {
  const id = useId();
  const invalidate = useSettingsInvalidation();
  const stored = settings.sources.find((source) => source.provider === provider);
  const [draft, setDraft] = useState<{ read: boolean; baseUrl: string } | null>(null);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const save = useSetSource(
    saving(() => {
      setDraft(null);
      setEmail("");
      setPassword("");
    }, invalidate),
  );

  const values = draft ?? {
    read: stored !== undefined,
    baseUrl: stored?.baseUrl ?? PROVIDER_BASE_URLS[provider],
  };
  const label = providerLabel(provider);

  return (
    <Section
      title={label}
      description="A source is read with an account of its own, and the address is both what is read and what a route is linked back to."
      save={save}
      edited={draft !== null}
      onSave={() =>
        save.mutate({
          provider,
          data: {
            read: values.read,
            baseUrl: values.baseUrl,
            ...replacement("email", email),
            ...replacement("password", password),
          },
        })
      }
    >
      <Field orientation="horizontal">
        <FieldContent>
          <FieldTitle>Read this library</FieldTitle>
        </FieldContent>
        <Switch
          checked={values.read}
          aria-label={`Read ${label}`}
          onCheckedChange={(read) => setDraft({ ...values, read })}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-url`}>Address</FieldLabel>
        <Input
          id={`${id}-url`}
          type="url"
          value={values.baseUrl}
          onChange={(event) => setDraft({ ...values, baseUrl: event.target.value })}
        />
      </Field>
      <SecretField
        id={`${id}-email`}
        label={`${label} email`}
        isSet={settings.secretsSet[`${provider}.email`] ?? false}
        value={email}
        onChange={setEmail}
      />
      <SecretField
        id={`${id}-password`}
        label={`${label} password`}
        isSet={settings.secretsSet[`${provider}.password`] ?? false}
        value={password}
        onChange={setPassword}
      />
    </Section>
  );
}

function Notifications({ settings }: { settings: Settings }) {
  const id = useId();
  const invalidate = useSettingsInvalidation();
  const [draft, setDraft] = useState<Settings["notifications"] | null>(null);
  const [token, setToken] = useState("");
  const [userKey, setUserKey] = useState("");
  const save = useSetNotifications(
    saving(() => {
      setDraft(null);
      setToken("");
      setUserKey("");
    }, invalidate),
  );

  const values = draft ?? settings.notifications;
  const edit = (change: Partial<Settings["notifications"]>) => setDraft({ ...values, ...change });

  return (
    <Section
      title="Notifications"
      save={save}
      edited={draft !== null}
      onSave={() =>
        save.mutate({
          data: {
            ...values,
            ...replacement("applicationToken", token),
            ...replacement("userKey", userKey),
          },
        })
      }
    >
      <Field orientation="horizontal">
        <FieldContent>
          <FieldTitle>Send notifications</FieldTitle>
          <FieldDescription>
            Off silences the whole channel, not only the routine ones: while it is off a failed run
            and a stale library go unsent as surely as a success does.
          </FieldDescription>
        </FieldContent>
        <Switch
          checked={values.enabled}
          aria-label="Send notifications"
          onCheckedChange={(enabled) => edit({ enabled })}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-pushover`}>Pushover address</FieldLabel>
        <Input
          id={`${id}-pushover`}
          type="url"
          value={values.pushoverBaseUrl}
          onChange={(event) => edit({ pushoverBaseUrl: event.target.value })}
        />
        <FieldDescription>
          The origin the application token and user key are sent to.
        </FieldDescription>
      </Field>
      <SecretField
        id={`${id}-token`}
        label="Pushover application token"
        isSet={settings.secretsSet["notifications.pushover.application_token"] ?? false}
        value={token}
        onChange={setToken}
      />
      <SecretField
        id={`${id}-user`}
        label="Pushover user key"
        isSet={settings.secretsSet["notifications.pushover.user_key"] ?? false}
        value={userKey}
        onChange={setUserKey}
      />
    </Section>
  );
}

/** The zone this service reads local time in — one for the whole service, not one per reader. */
function Timezone({ settings }: { settings: Settings }) {
  const id = useId();
  const invalidate = useSettingsInvalidation();
  const [draft, setDraft] = useState<string | null>(null);
  const save = useSetTimezone(saving(() => setDraft(null), invalidate));

  const value = draft ?? settings.timezone;

  return (
    <Section
      title="Timezone"
      save={save}
      edited={draft !== null}
      onSave={() => save.mutate({ data: { timezone: value } })}
    >
      <Field>
        <FieldLabel htmlFor={`${id}-timezone`}>IANA zone</FieldLabel>
        <Input
          id={`${id}-timezone`}
          value={value}
          onChange={(event) => setDraft(event.target.value)}
        />
        <FieldDescription>
          What a scheduled time of day means, and what hour a forecast describes. A zone this
          service cannot load is refused.
        </FieldDescription>
      </Field>
    </Section>
  );
}

/** One alert's place in the matrix, and the key its pending edit is held under. */
function alertKey(alert: Pick<AlertSetting, "task" | "alert">): string {
  return `${alert.task}/${alert.alert}`;
}

/** The reason an alert names, as prose rather than as the stored slug. */
function alertLabel(alert: AlertSetting): string {
  return alert.alert.replaceAll("_", " ");
}

/**
 * The matrix is what this build declares, not what has been decided: an alert
 * nobody has ruled on defaults to on. Only switches this card has moved are sent.
 */
function Alerts({ settings }: { settings: Settings }) {
  const invalidate = useSettingsInvalidation();
  const [draft, setDraft] = useState<Record<string, boolean>>({});
  const save = useSetAlerts(saving(() => setDraft({}), invalidate));

  const tasks = [...new Set(settings.alerts.map((alert) => alert.task))];

  return (
    <Section
      title="Alerts"
      description="What the service announces when it goes wrong, one switch per reason. Turning the whole channel off above silences every one of these regardless."
      save={save}
      edited={Object.keys(draft).length > 0}
      onSave={() =>
        save.mutate({
          data: {
            alerts: settings.alerts
              .filter((alert) => alertKey(alert) in draft)
              .map((alert) => ({
                task: alert.task,
                alert: alert.alert,
                enabled: draft[alertKey(alert)] ?? alert.enabled,
              })),
          },
        })
      }
    >
      {tasks.length === 0 ? (
        <FieldDescription>This build announces nothing.</FieldDescription>
      ) : (
        tasks.map((taskName) => (
          <FieldSet key={taskName}>
            <FieldLegend>{taskName}</FieldLegend>
            <FieldGroup>
              {settings.alerts
                .filter((alert) => alert.task === taskName)
                .map((alert) => (
                  <Field key={alertKey(alert)} orientation="horizontal">
                    <FieldContent>
                      <FieldTitle>{alertLabel(alert)}</FieldTitle>
                    </FieldContent>
                    <Switch
                      checked={draft[alertKey(alert)] ?? alert.enabled}
                      aria-label={`${taskName} ${alertLabel(alert)}`}
                      onCheckedChange={(enabled) =>
                        setDraft((pending) => {
                          // A switch put back where it started isn't a decision worth sending.
                          const { [alertKey(alert)]: _, ...rest } = pending;

                          return enabled === alert.enabled
                            ? rest
                            : { ...rest, [alertKey(alert)]: enabled };
                        })
                      }
                    />
                  </Field>
                ))}
            </FieldGroup>
          </FieldSet>
        ))
      )}
    </Section>
  );
}

/**
 * Past this many styles, the rows carry names alone.
 *
 * Every strip is a WebGL context and a browser hands out about sixteen for the
 * whole page; an entry with a dark style draws two. See `BasemapPreview`.
 */
const MOST_STRIPS = 12;

function Basemaps({ settings }: { settings: Settings }) {
  const id = useId();
  const invalidate = useSettingsInvalidation();
  const [draft, setDraft] = useState<BrowserBasemap[] | null>(null);
  // A basemap row has no identity in the settings — two rows are both blank
  // while they are being typed — so one is kept beside them, or removing the
  // first row would move every value below it up into a different input.
  const [rowKeys, setRowKeys] = useState(() => settings.basemaps.map((_, index) => index));
  const nextRowKey = useRef(settings.basemaps.length);
  const [openKeys, setOpenKeys] = useState<number[]>([]);
  const save = useSetBasemaps(saving(() => setDraft(null), invalidate));

  const basemaps = draft ?? settings.basemaps;
  const replaceBasemap = (index: number, next: BrowserBasemap) =>
    setDraft(basemaps.map((basemap, at) => (at === index ? next : basemap)));

  // The browser may only reach the origins of saved styles, and the hosts each
  // one names are learnt on save — so a typed URL cannot be drawn until then.
  const saved = new Set(
    settings.basemaps.flatMap((basemap) => [basemap.styleUrl, basemap.styleUrlDark]),
  );
  const strips = basemaps.reduce((count, basemap) => count + (basemap.styleUrlDark ? 2 : 1), 0);
  const styleUrlsOf = (basemap: BrowserBasemap) =>
    strips <= MOST_STRIPS
      ? [basemap.styleUrl, basemap.styleUrlDark].filter((url): url is string => Boolean(url))
      : [];

  return (
    <Section
      title="Basemaps"
      description="The cartography this page offers. An entry with a dark style switches between the two with the system colour scheme; an entry whose own ground is dark whatever the scheme is — imagery — says so instead, and the two cannot both be set. A preview shows a style as last saved."
      save={save}
      edited={draft !== null}
      onSave={() => save.mutate({ data: { basemaps } })}
    >
      {basemaps.map((basemap, index) => {
        const key = rowKeys[index] ?? index;
        const open = openKeys.includes(key);

        return (
          <Collapsible
            key={key}
            open={open}
            onOpenChange={(next) =>
              setOpenKeys(next ? [...openKeys, key] : openKeys.filter((other) => other !== key))
            }
            className="grid gap-3 rounded-lg border border-[var(--rule)] p-3"
          >
            <div className="flex items-center justify-between gap-3">
              <span className="font-medium">{basemap.name || `Basemap ${index + 1}`}</span>
              <CollapsibleTrigger
                render={<Button variant="outline" />}
                aria-label={`${open ? "Finish editing" : "Edit"} basemap ${index + 1}`}
              >
                {open ? "Done" : "Edit"}
              </CollapsibleTrigger>
            </div>
            {styleUrlsOf(basemap).length > 0 ? (
              <div className="grid gap-3 sm:grid-flow-col sm:auto-cols-fr">
                {styleUrlsOf(basemap).map((url) =>
                  saved.has(url) ? (
                    <BasemapStrip key={url} styleUrl={url} />
                  ) : (
                    <span
                      key={url}
                      className="flex h-40 items-center justify-center rounded-lg bg-[var(--base)] text-sm text-[var(--ink-2)] ring-1 ring-[var(--rule)]"
                    >
                      Save to preview
                    </span>
                  ),
                )}
              </div>
            ) : null}
            <CollapsibleContent className="grid gap-3">
              <Field>
                <FieldLabel htmlFor={`${id}-name-${key}`}>Name</FieldLabel>
                <Input
                  id={`${id}-name-${key}`}
                  value={basemap.name}
                  onChange={(event) =>
                    replaceBasemap(index, { ...basemap, name: event.target.value })
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${id}-style-${key}`}>Style URL</FieldLabel>
                <Input
                  id={`${id}-style-${key}`}
                  type="url"
                  value={basemap.styleUrl}
                  onChange={(event) =>
                    replaceBasemap(index, { ...basemap, styleUrl: event.target.value })
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${id}-dark-${key}`}>Dark style URL (optional)</FieldLabel>
                <Input
                  id={`${id}-dark-${key}`}
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
                  variant="destructive"
                  disabled={basemaps.length === 1}
                  aria-label={`Remove basemap ${index + 1}`}
                  onClick={() => {
                    setDraft(basemaps.filter((_, at) => at !== index));
                    setRowKeys(rowKeys.filter((_, at) => at !== index));
                  }}
                >
                  Remove
                </Button>
              </div>
            </CollapsibleContent>
          </Collapsible>
        );
      })}
      <div>
        <Button
          variant="outline"
          onClick={() => {
            setDraft([...basemaps, { name: "", styleUrl: "" }]);
            setRowKeys([...rowKeys, nextRowKey.current]);
            // A new entry has nothing saved to show, so it opens onto its fields.
            setOpenKeys([...openKeys, nextRowKey.current]);
            nextRowKey.current += 1;
          }}
        >
          Add a basemap
        </Button>
      </div>
    </Section>
  );
}

function SurfaceClassification({ settings }: { settings: Settings }) {
  const id = useId();
  const invalidate = useSettingsInvalidation();
  const [draft, setDraft] = useState<Settings["surface"] | null>(null);
  const save = useSetSurface(saving(() => setDraft(null), invalidate));

  const values = draft ?? settings.surface;

  return (
    <Section
      title="Surface classification"
      save={save}
      edited={draft !== null}
      onSave={() => save.mutate({ data: values })}
    >
      <Field>
        <RegionPicker
          value={values.regions}
          onChange={(regions) => setDraft({ ...values, regions })}
        />
        <FieldDescription>
          Choosing no region switches classification off. Naming one does not build the index: the
          next rebuild on the schedule below does, and routes are classified on the pass after that.
        </FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-rebuild`}>Rebuild the index every (hours)</FieldLabel>
        <Input
          id={`${id}-rebuild`}
          type="number"
          min={1}
          step="any"
          value={inHours(values.rebuildIntervalSeconds)}
          onChange={(event) =>
            setDraft({ ...values, rebuildIntervalSeconds: fromHours(Number(event.target.value)) })
          }
        />
        <FieldDescription>
          Required whether or not a region is named: the schedule runs either way, and with no
          region it builds nothing.
        </FieldDescription>
      </Field>
    </Section>
  );
}

function RideModel({ settings }: { settings: Settings }) {
  const id = useId();
  const invalidate = useSettingsInvalidation();
  const [draft, setDraft] = useState<string | null>(null);
  const save = useSetRideModel(saving(() => setDraft(null), invalidate));

  const coefficientsFile = draft ?? settings.rideModel.coefficientsFile;

  return (
    <Section
      title="Ride model"
      save={save}
      edited={draft !== null}
      onSave={() => save.mutate({ data: { coefficientsFile } })}
    >
      <Field>
        <FieldLabel htmlFor={`${id}-coefficients`}>Coefficient file</FieldLabel>
        <Input
          id={`${id}-coefficients`}
          value={coefficientsFile}
          onChange={(event) => setDraft(event.target.value)}
        />
        <FieldDescription>
          An absolute path, on the machine the service runs on, to the file the fitting tooling
          produced. Empty leaves routes without a predicted moving time.
        </FieldDescription>
      </Field>
    </Section>
  );
}

function Sync({ settings }: { settings: Settings }) {
  const id = useId();
  const invalidate = useSettingsInvalidation();
  const [draft, setDraft] = useState<Settings["sync"] | null>(null);
  const [confirmingDeletion, setConfirmingDeletion] = useState(false);
  const save = useSetSync(saving(() => setDraft(null), invalidate));

  const values = draft ?? settings.sync;
  const edit = (change: Partial<Settings["sync"]>) => setDraft({ ...values, ...change });

  return (
    <Section
      title="Sync"
      save={save}
      edited={draft !== null}
      onSave={() => save.mutate({ data: values })}
    >
      <Field orientation="horizontal">
        <FieldContent>
          <FieldTitle>Let an empty library delete a target's routes</FieldTitle>
          <FieldDescription>
            A read that finds nothing at the source is otherwise treated as a fault and the write is
            held. This stays on until you turn it off again — it does not reset after one run.
          </FieldDescription>
        </FieldContent>
        <Switch
          checked={values.allowEmptySourceDeletion}
          aria-label="Let an empty library delete a target's routes"
          onCheckedChange={(next) =>
            next ? setConfirmingDeletion(true) : edit({ allowEmptySourceDeletion: false })
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
          value={inHours(values.staleAfterSeconds)}
          onChange={(event) => edit({ staleAfterSeconds: fromHours(Number(event.target.value)) })}
        />
        <FieldDescription>
          How long the last successful read may stand before the status page reports the inventory
          as stale, and says so.
        </FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-initial-delay`}>Wait before the first run (minutes)</FieldLabel>
        <Input
          id={`${id}-initial-delay`}
          type="number"
          min={1}
          step="any"
          value={inMinutes(values.initialDelaySeconds)}
          onChange={(event) =>
            edit({ initialDelaySeconds: fromMinutes(Number(event.target.value)) })
          }
        />
        <FieldDescription>
          Read by the start it delays, so this one takes effect on the next restart rather than the
          next run.
        </FieldDescription>
      </Field>

      {/*
       * The one switch on this page that asks first. It is the one that lets a
       * sync delete an entire library, and it is a switch rather than
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
            <AlertDialogCancel render={<Button variant="outline" />}>Cancel</AlertDialogCancel>
            {/*
             * Closes as well as edits: the dialog is controlled, and neither
             * this nor the `AlertDialogAction` it replaced is a `Close`, so
             * nothing else would put it away after the reader has answered.
             */}
            <Button
              variant="destructive"
              onClick={() => {
                edit({ allowEmptySourceDeletion: true });
                setConfirmingDeletion(false);
              }}
            >
              Allow it
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Section>
  );
}
