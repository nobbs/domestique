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

import {
  IconAlertTriangle,
  IconBell,
  IconClockHour4,
  IconDatabase,
  IconMap,
  IconPlugConnected,
  IconRefresh,
  IconRoad,
  IconSettings,
  IconWorld,
} from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, type ReactNode, useId, useRef, useState } from "react";
import {
  FormFooter,
  FormGroup,
  FormRow,
  InfoDot,
  RowInput,
  SecretInput,
  UnitInput,
} from "@/components/InsetForm";
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
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { FieldDescription } from "@/components/ui/field";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import {
  useSetAlerts,
  useSetBasemaps,
  useSetNotifications,
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
import { Panel } from "../../components/PanelHeading";
import { formatCount } from "../../lib/format";
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
  srLabel,
  isSet,
  value,
  onChange,
}: {
  id: string;
  label: string;
  srLabel?: string;
  isSet: boolean;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <FormRow label={label} srLabel={srLabel} htmlFor={id}>
      <SecretInput
        id={id}
        isSet={isSet}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </FormRow>
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
  icon,
  description,
  save,
  onSave,
  edited,
  children,
}: {
  title: string;
  icon: ReactNode;
  description?: ReactNode;
  save: SaveState;
  onSave: () => void;
  edited: boolean;
  children: ReactNode;
}) {
  return (
    <Panel
      icon={icon}
      title={title}
      level={3}
      {...(description
        ? {
            aside: (
              <InfoDot label={title} framed>
                {description}
              </InfoDot>
            ),
          }
        : {})}
    >
      <form
        className="grid gap-6"
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          onSave();
        }}
      >
        {children}
        <FormFooter>
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
          <Button
            variant="default"
            aria-label={`Save ${title}`}
            disabled={save.isPending}
            onClick={onSave}
          >
            {save.isPending ? <Spinner aria-label="Saving" /> : null}
            Save
          </Button>
        </FormFooter>
      </form>
    </Panel>
  );
}

/** The card chrome for the two states before there are any settings to show. */
function SettingsCard({ children }: { children: ReactNode }) {
  return (
    <Panel icon={<IconSettings size={18} stroke={1.8} />} title="Service settings" level={3}>
      <div className="grid gap-8">{children}</div>
    </Panel>
  );
}

/** The areas Admin splits the service settings into, one tab each. */
export type SettingsGroup = "service" | "integrations" | "alerting" | "map";

/** The service settings of one group, or every group with the missing list above them. */
export function ServiceSettings({ group }: { group?: SettingsGroup }) {
  const { data, isPending, isError } = useQuery(settingsQuery());

  const Wrap = group ? SettingsCard : Sections;
  if (isPending) {
    return (
      <Wrap>
        <Skeleton className="h-64 w-full" role="status" aria-label="Loading service settings" />
      </Wrap>
    );
  }
  if (isError) {
    return (
      <Wrap>
        <p className="text-sm text-[var(--alert)]" role="alert">
          The service did not say what it is set to.
        </p>
      </Wrap>
    );
  }

  const groups: Record<SettingsGroup, ReactNode> = {
    service: (
      <>
        <Timezone settings={data} />
        <Sync settings={data} />
        <RideModel settings={data} />
      </>
    ),
    integrations: (
      <>
        <WahooApplication settings={data} />
        {SOURCE_PROVIDERS.map((provider) => (
          <SourceSettingsSection key={provider} provider={provider} settings={data} />
        ))}
      </>
    ),
    alerting: (
      <>
        <Notifications settings={data} />
        <Alerts settings={data} />
      </>
    ),
    map: (
      <>
        <Basemaps settings={data} />
        <SurfaceClassification settings={data} />
      </>
    ),
  };

  if (group) {
    return <div className="grid gap-6">{groups[group]}</div>;
  }

  return (
    <Sections>
      <Missing missing={data.missing} />
      {groups.service}
      {groups.integrations}
      {groups.alerting}
      {groups.map}
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
export function Missing({ missing }: { missing: string[] }) {
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
  const [webhookToken, setWebhookToken] = useState("");
  const save = useSetWahooApplication(
    saving(() => {
      setDraft(null);
      setSecret("");
      setWebhookToken("");
    }, invalidate),
  );

  const values = draft ?? settings.wahoo;
  const edit = (change: Partial<Settings["wahoo"]>) => setDraft({ ...values, ...change });

  return (
    <Section
      title="Wahoo application"
      icon={<IconPlugConnected size={18} stroke={1.8} />}
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
            ...replacement("webhookToken", webhookToken),
          },
        })
      }
    >
      <FormGroup title="Endpoints">
        <FormRow label="API address" htmlFor={`${id}-api`}>
          <RowInput
            id={`${id}-api`}
            type="url"
            value={values.apiBaseUrl}
            onChange={(event) => edit({ apiBaseUrl: event.target.value })}
          />
        </FormRow>
        <FormRow label="Authorization address" htmlFor={`${id}-oauth`}>
          <RowInput
            id={`${id}-oauth`}
            type="url"
            value={values.oauthBaseUrl}
            onChange={(event) => edit({ oauthBaseUrl: event.target.value })}
          />
        </FormRow>
      </FormGroup>
      <FormGroup title="Credentials">
        <FormRow label="Client ID" htmlFor={`${id}-client`}>
          <RowInput
            id={`${id}-client`}
            value={values.clientId}
            onChange={(event) => edit({ clientId: event.target.value })}
          />
        </FormRow>
        <SecretField
          id={`${id}-secret`}
          label="Client secret"
          isSet={settings.secretsSet["wahoo.client_secret"] ?? false}
          value={secret}
          onChange={setSecret}
        />
        <SecretField
          id={`${id}-webhook`}
          label="Webhook token"
          isSet={settings.secretsSet["wahoo.webhook_token"] ?? false}
          value={webhookToken}
          onChange={setWebhookToken}
        />
      </FormGroup>
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
  const [draft, setDraft] = useState<{
    read: boolean;
    syncToWahoo: boolean;
    baseUrl: string;
  } | null>(null);
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
    syncToWahoo: stored?.syncToWahoo ?? true,
    baseUrl: stored?.baseUrl ?? PROVIDER_BASE_URLS[provider],
  };
  const label = providerLabel(provider);

  return (
    <Section
      title={label}
      icon={<IconDatabase size={18} stroke={1.8} />}
      description="A source is read with an account of its own, and the address is both what is read and what a route is linked back to."
      save={save}
      edited={draft !== null}
      onSave={() =>
        save.mutate({
          provider,
          data: {
            read: values.read,
            syncToWahoo: values.syncToWahoo,
            baseUrl: values.baseUrl,
            ...replacement("email", email),
            ...replacement("password", password),
          },
        })
      }
    >
      <FormGroup>
        <FormRow
          label="Sync to catalogue"
          hint="Off removes the library's routes from the catalogue at the next library sync, and from every rider's Wahoo account up to five per run."
        >
          <Switch
            checked={values.read}
            aria-label={`Sync ${label} to catalogue`}
            onCheckedChange={(read) => setDraft({ ...values, read })}
          />
        </FormRow>
        <FormRow
          label="Sync to Wahoo"
          hint="Off keeps the library in the catalogue but off every rider's Wahoo account; routes already there are removed, up to five per run. Needs catalogue sync on."
        >
          <Switch
            checked={values.syncToWahoo}
            disabled={!values.read}
            aria-label={`Sync ${label} to Wahoo`}
            onCheckedChange={(syncToWahoo) => setDraft({ ...values, syncToWahoo })}
          />
        </FormRow>
        <FormRow label="Address" htmlFor={`${id}-url`}>
          <RowInput
            id={`${id}-url`}
            type="url"
            value={values.baseUrl}
            onChange={(event) => setDraft({ ...values, baseUrl: event.target.value })}
          />
        </FormRow>
        <SecretField
          id={`${id}-email`}
          label="Email"
          srLabel={`(${label})`}
          isSet={settings.secretsSet[`${provider}.email`] ?? false}
          value={email}
          onChange={setEmail}
        />
        <SecretField
          id={`${id}-password`}
          label="Password"
          srLabel={`(${label})`}
          isSet={settings.secretsSet[`${provider}.password`] ?? false}
          value={password}
          onChange={setPassword}
        />
      </FormGroup>
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
      icon={<IconBell size={18} stroke={1.8} />}
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
      <FormGroup>
        <FormRow
          label="Send notifications"
          hint="Off silences the whole channel, not only the routine ones: while it is off a failed run and a stale library go unsent as surely as a success does."
        >
          <Switch
            checked={values.enabled}
            aria-label="Send notifications"
            onCheckedChange={(enabled) => edit({ enabled })}
          />
        </FormRow>
        <FormRow
          label="Pushover address"
          htmlFor={`${id}-pushover`}
          hint="The origin the application token and user key are sent to."
        >
          <RowInput
            id={`${id}-pushover`}
            type="url"
            value={values.pushoverBaseUrl}
            onChange={(event) => edit({ pushoverBaseUrl: event.target.value })}
          />
        </FormRow>
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
      </FormGroup>
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
      icon={<IconWorld size={18} stroke={1.8} />}
      save={save}
      edited={draft !== null}
      onSave={() => save.mutate({ data: { timezone: value } })}
    >
      <FormGroup>
        <FormRow
          label="IANA zone"
          htmlFor={`${id}-timezone`}
          hint="What a scheduled time of day means, and what hour a forecast describes. A zone this service cannot load is refused."
        >
          <RowInput
            id={`${id}-timezone`}
            value={value}
            onChange={(event) => setDraft(event.target.value)}
          />
        </FormRow>
      </FormGroup>
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
      icon={<IconAlertTriangle size={18} stroke={1.8} />}
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
          <FormGroup key={taskName} title={taskName}>
            {settings.alerts
              .filter((alert) => alert.task === taskName)
              .map((alert) => (
                <FormRow key={alertKey(alert)} label={alertLabel(alert)}>
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
                </FormRow>
              ))}
          </FormGroup>
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
  // Once saved, every row is an original again, so its key is its index.
  const save = useSetBasemaps(
    saving(() => {
      setDraft(null);
      setRowKeys((keys) => keys.map((_, index) => index));
      setOpenKeys([]);
    }, invalidate),
  );

  const basemaps = draft ?? settings.basemaps;
  const replaceBasemap = (index: number, next: BrowserBasemap) =>
    setDraft(basemaps.map((basemap, at) => (at === index ? next : basemap)));

  // The browser may only reach the origins of saved styles, and the hosts each
  // one names are learnt on save — so a typed URL cannot be drawn until then.
  const slotsOf = (key: number, basemap: BrowserBasemap) => {
    const saved = settings.basemaps[key];

    return [
      { role: "light", url: basemap.styleUrl, saved: basemap.styleUrl === saved?.styleUrl },
      {
        role: "dark",
        url: basemap.styleUrlDark,
        saved: basemap.styleUrlDark === saved?.styleUrlDark,
      },
    ].filter((slot): slot is { role: string; url: string; saved: boolean } => Boolean(slot.url));
  };
  // Only a saved slot draws a map; a placeholder costs no context.
  const strips = basemaps.reduce(
    (count, basemap, index) =>
      count + slotsOf(rowKeys[index] ?? index, basemap).filter((slot) => slot.saved).length,
    0,
  );
  const styleUrlsOf = (key: number, basemap: BrowserBasemap) =>
    strips <= MOST_STRIPS ? slotsOf(key, basemap) : [];

  return (
    <Section
      title="Basemaps"
      icon={<IconMap size={18} stroke={1.8} />}
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
              setOpenKeys((keys) =>
                next
                  ? [...keys.filter((other) => other !== key), key]
                  : keys.filter((other) => other !== key),
              )
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
            {styleUrlsOf(key, basemap).length > 0 ? (
              <div className="grid gap-3 sm:grid-flow-col sm:auto-cols-fr">
                {styleUrlsOf(key, basemap).map(({ role, url, saved }) =>
                  saved ? (
                    <BasemapStrip key={role} styleUrl={url} />
                  ) : (
                    <span
                      key={role}
                      className="flex h-40 items-center justify-center rounded-lg bg-[var(--base)] text-sm text-[var(--ink-2)] ring-1 ring-[var(--rule)]"
                    >
                      Save to preview
                    </span>
                  ),
                )}
              </div>
            ) : null}
            <CollapsibleContent className="grid gap-3">
              <FormGroup>
                <FormRow label="Name" htmlFor={`${id}-name-${key}`}>
                  <RowInput
                    id={`${id}-name-${key}`}
                    value={basemap.name}
                    onChange={(event) =>
                      replaceBasemap(index, { ...basemap, name: event.target.value })
                    }
                  />
                </FormRow>
                <FormRow label="Style URL" htmlFor={`${id}-style-${key}`}>
                  <RowInput
                    id={`${id}-style-${key}`}
                    type="url"
                    value={basemap.styleUrl}
                    onChange={(event) =>
                      replaceBasemap(index, { ...basemap, styleUrl: event.target.value })
                    }
                  />
                </FormRow>
                <FormRow label="Dark style URL (optional)" htmlFor={`${id}-dark-${key}`}>
                  <RowInput
                    id={`${id}-dark-${key}`}
                    type="url"
                    value={basemap.styleUrlDark ?? ""}
                    onChange={(event) =>
                      replaceBasemap(index, withDarkStyle(basemap, event.target.value))
                    }
                  />
                </FormRow>
                <FormRow label="This style is dark cartography">
                  <Switch
                    checked={basemap.darkCartography ?? false}
                    disabled={Boolean(basemap.styleUrlDark)}
                    aria-label={`This style is dark cartography: basemap ${index + 1}`}
                    onCheckedChange={(darkCartography) =>
                      replaceBasemap(index, { ...basemap, darkCartography })
                    }
                  />
                </FormRow>
              </FormGroup>
              <div>
                <Button
                  variant="destructive"
                  disabled={basemaps.length === 1}
                  aria-label={`Remove basemap ${index + 1}`}
                  onClick={() => {
                    setDraft(basemaps.filter((_, at) => at !== index));
                    setRowKeys(rowKeys.filter((_, at) => at !== index));
                    setOpenKeys((keys) => keys.filter((other) => other !== key));
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
            const added = nextRowKey.current;
            setRowKeys([...rowKeys, added]);
            // A new entry has nothing saved to show, so it opens onto its fields.
            setOpenKeys((keys) => [...keys, added]);
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
      icon={<IconRoad size={18} stroke={1.8} />}
      save={save}
      edited={draft !== null}
      onSave={() => save.mutate({ data: values })}
    >
      {/* RegionPicker owns its own label, chips and search field, not a single control a row can hold. */}
      <div className="grid gap-2">
        <RegionPicker
          value={values.regions}
          onChange={(regions) => setDraft({ ...values, regions })}
        />
        <FieldDescription>
          Choosing no region switches classification off. Naming one does not build the index: the
          next rebuild on the schedule below does, and routes are classified on the pass after that.
        </FieldDescription>
      </div>
      <FormGroup>
        <FormRow
          label="Rebuild the index every"
          srLabel="(hours)"
          htmlFor={`${id}-rebuild`}
          hint="Required whether or not a region is named: the schedule runs either way, and with no region it builds nothing."
        >
          <UnitInput
            id={`${id}-rebuild`}
            unit="hours"
            type="number"
            min={1}
            step="any"
            value={inHours(values.rebuildIntervalSeconds)}
            onChange={(event) =>
              setDraft({ ...values, rebuildIntervalSeconds: fromHours(Number(event.target.value)) })
            }
          />
        </FormRow>
      </FormGroup>
    </Section>
  );
}

function RideModel({ settings }: { settings: Settings }) {
  const model = settings.rideModel;
  const cutoff = model.calibrationCutoff;
  const provenance =
    model.source === "calibrated"
      ? `Calibrated${cutoff ? ` on ${cutoff}` : ""}${
          model.evaluatedRides > 0 ? ` from ${formatCount(model.evaluatedRides, "ride")}` : ""
        }.`
      : "Built-in default, in force until the first calibration succeeds.";

  return (
    <Panel icon={<IconClockHour4 size={18} stroke={1.8} />} title="Ride model" level={3}>
      <div className="grid gap-2">
        <dl className="grid gap-1 text-sm">
          <div className="flex justify-between gap-4">
            <dt className="text-[var(--ink-2)]">Seconds per kilometre</dt>
            <dd>{model.secondsPerKm.toFixed(2)}</dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt className="text-[var(--ink-2)]">Seconds per metre climbed</dt>
            <dd>{model.secondsPerAscentM.toFixed(2)}</dd>
          </div>
        </dl>
        <FieldDescription>{provenance}</FieldDescription>
      </div>
    </Panel>
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
      icon={<IconRefresh size={18} stroke={1.8} />}
      save={save}
      edited={draft !== null}
      onSave={() => save.mutate({ data: values })}
    >
      <FormGroup title="Timing">
        <FormRow
          label="Call the library stale after"
          srLabel="(hours)"
          htmlFor={`${id}-stale`}
          hint="How long the last successful read may stand before the status page reports the inventory as stale, and says so."
        >
          <UnitInput
            id={`${id}-stale`}
            unit="hours"
            type="number"
            min={1}
            step="any"
            value={inHours(values.staleAfterSeconds)}
            onChange={(event) => edit({ staleAfterSeconds: fromHours(Number(event.target.value)) })}
          />
        </FormRow>
        <FormRow
          label="Wait before the first run"
          srLabel="(minutes)"
          htmlFor={`${id}-initial-delay`}
          hint="Read by the start it delays, so this one takes effect on the next restart rather than the next run."
        >
          <UnitInput
            id={`${id}-initial-delay`}
            unit="minutes"
            type="number"
            min={1}
            step="any"
            value={inMinutes(values.initialDelaySeconds)}
            onChange={(event) =>
              edit({ initialDelaySeconds: fromMinutes(Number(event.target.value)) })
            }
          />
        </FormRow>
      </FormGroup>
      <FormGroup title="Safety">
        <FormRow
          label="Let an empty library delete a target's routes"
          tone="alert"
          hint="A read that finds nothing at the source is otherwise treated as a fault and the write is held. This stays on until you turn it off again — it does not reset after one run."
        >
          <Switch
            checked={values.allowEmptySourceDeletion}
            aria-label="Let an empty library delete a target's routes"
            onCheckedChange={(next) =>
              next ? setConfirmingDeletion(true) : edit({ allowEmptySourceDeletion: false })
            }
          />
        </FormRow>
      </FormGroup>

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
