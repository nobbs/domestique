/**
 * The rider's own Zwift account, on their own settings page.
 *
 * Zwift's own account credentials, not an OAuth token: there is no Zwift
 * application to register with, so the rider's email and password are stored
 * here, sealed, and never rendered back. A save carries only the boxes typed,
 * never a value already stored.
 */

import { IconDeviceGamepad2 } from "@tabler/icons-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { FormFooter, FormGroup, FormRow, InfoDot, SecretInput } from "@/components/InsetForm";
import { Panel } from "@/components/PanelHeading";
import { Spinner } from "@/components/ui/spinner";
import { useDeleteRiderZwiftCredentials, useSetRiderZwiftCredentials } from "../../api/generated";
import { riderProfileQuery } from "../../api/queries";
import { Button } from "../../components/Button";
import { Skeleton } from "../../components/ui/skeleton";

function CardShell({ children }: { children: React.ReactNode }) {
  return (
    <Panel
      icon={<IconDeviceGamepad2 size={18} stroke={1.8} />}
      title="Zwift account"
      aside={
        <InfoDot label="Zwift account" framed>
          Zwift's API is unofficial and not supported by Zwift; your own account's terms of service
          still apply to reading it this way.
        </InfoDot>
      }
    >
      <div className="grid gap-3">{children}</div>
    </Panel>
  );
}

export function ZwiftAccountCard() {
  const id = useId();
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(riderProfileQuery());
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: riderProfileQuery().queryKey });
  const save = useSetRiderZwiftCredentials({
    mutation: {
      onSuccess: async () => {
        setEmail("");
        setPassword("");
        await invalidate();
      },
    },
  });
  const disconnect = useDeleteRiderZwiftCredentials({
    mutation: { onSuccess: () => invalidate() },
  });

  if (isPending) {
    return (
      <CardShell>
        <Skeleton className="h-40 w-full" role="status" aria-label="Loading your Zwift account" />
      </CardShell>
    );
  }
  if (isError) {
    return (
      <CardShell>
        <p className="text-sm text-[var(--alert)]" role="alert">
          The service did not say what your Zwift account holds.
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
              isSet={data.zwift.emailSet}
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </FormRow>
          <FormRow label="Password" htmlFor={`${id}-password`}>
            <SecretInput
              id={`${id}-password`}
              isSet={data.zwift.passwordSet}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </FormRow>
        </FormGroup>
        <FormFooter>
          {save.isError ? (
            <p className="text-sm text-[var(--alert)]" role="alert">
              Your Zwift account was not saved.
            </p>
          ) : null}
          {disconnect.isError ? (
            <p className="text-sm text-[var(--alert)]" role="alert">
              Your Zwift account was not disconnected.
            </p>
          ) : null}
          <Button
            variant="outline"
            aria-label="Disconnect Zwift account"
            disabled={disconnect.isPending || (!data.zwift.emailSet && !data.zwift.passwordSet)}
            onClick={() => disconnect.mutate()}
          >
            {disconnect.isPending ? <Spinner aria-label="Disconnecting" /> : null}
            Disconnect
          </Button>
          <Button
            variant="default"
            aria-label="Save Zwift account"
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
