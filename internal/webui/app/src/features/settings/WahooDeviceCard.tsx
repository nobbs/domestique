/**
 * The rider's own Wahoo email and password, used only to sign in the way an
 * ELEMNT does and give each synced route the identity the device keys it by.
 */

import { IconDeviceWatch } from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { FormFooter, FormGroup, FormRow, InfoDot, SecretInput } from "@/components/InsetForm";
import { Panel } from "@/components/PanelHeading";
import { Spinner } from "@/components/ui/spinner";
import { useDeleteRiderWahooCredentials, useSetRiderWahooCredentials } from "../../api/generated";
import { riderProfileQuery } from "../../api/queries";
import { Button } from "../../components/Button";
import { Skeleton } from "../../components/ui/skeleton";

function CardShell({ children }: { children: React.ReactNode }) {
  return (
    <Panel
      icon={<IconDeviceWatch size={18} stroke={1.8} />}
      title="Wahoo device sign-in"
      aside={
        <InfoDot label="Wahoo device routes" framed>
          Wahoo's public API cannot give a route the identity an ELEMNT keys it by, so without this
          every route lands in one slot on the device and they overwrite each other. With your Wahoo
          email and password the service signs in the way your ELEMNT does, only to label your
          routes. The device API is unofficial.
        </InfoDot>
      }
    >
      <div className="grid gap-3">{children}</div>
    </Panel>
  );
}

export function WahooDeviceCard() {
  const id = useId();
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(riderProfileQuery());
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: riderProfileQuery().queryKey });
  const save = useSetRiderWahooCredentials({
    mutation: {
      onSuccess: async () => {
        setEmail("");
        setPassword("");
        await invalidate();
      },
    },
  });
  const disconnect = useDeleteRiderWahooCredentials({
    mutation: { onSuccess: () => invalidate() },
  });

  if (isPending) {
    return (
      <CardShell>
        <Skeleton
          className="h-40 w-full"
          role="status"
          aria-label="Loading your Wahoo device sign-in"
        />
      </CardShell>
    );
  }
  if (isError) {
    return (
      <CardShell>
        <p className="text-sm text-[var(--alert)]" role="alert">
          The service did not say what your Wahoo device sign-in holds.
        </p>
      </CardShell>
    );
  }

  const onSave = () => {
    const edited: { email?: string; password?: string } = {};
    if (email !== "") {
      edited.email = email;
    }
    if (password !== "") {
      edited.password = password;
    }
    save.mutate({ data: edited });
  };

  return (
    <CardShell>
      <form
        className="grid gap-5"
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          onSave();
        }}
      >
        <FormGroup>
          <FormRow label="Email" htmlFor={`${id}-email`}>
            <SecretInput
              id={`${id}-email`}
              isSet={data.wahoo.emailSet}
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </FormRow>
          <FormRow label="Password" htmlFor={`${id}-password`}>
            <SecretInput
              id={`${id}-password`}
              isSet={data.wahoo.passwordSet}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </FormRow>
        </FormGroup>
        <FormFooter>
          {data.wahoo.signInRefused ? (
            <p className="text-sm text-[var(--alert)]" role="alert">
              Wahoo refused this email and password at the last sync. Re-enter them to resume.
            </p>
          ) : null}
          {save.isError ? (
            <p className="text-sm text-[var(--alert)]" role="alert">
              Your Wahoo device sign-in was not saved.
            </p>
          ) : null}
          {disconnect.isError ? (
            <p className="text-sm text-[var(--alert)]" role="alert">
              Your Wahoo device sign-in was not disconnected.
            </p>
          ) : null}
          <Button
            variant="outline"
            aria-label="Disconnect Wahoo device sign-in"
            disabled={disconnect.isPending || (!data.wahoo.emailSet && !data.wahoo.passwordSet)}
            onClick={() => disconnect.mutate()}
          >
            {disconnect.isPending ? <Spinner aria-label="Disconnecting" /> : null}
            Disconnect
          </Button>
          <Button
            variant="default"
            aria-label="Save Wahoo device sign-in"
            disabled={save.isPending || (email === "" && password === "")}
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
