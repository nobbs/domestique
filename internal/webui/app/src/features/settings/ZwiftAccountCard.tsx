/**
 * The rider's own Zwift account, on their own settings page.
 *
 * Zwift's own account credentials, not an OAuth token: there is no Zwift
 * application to register with, so the rider's email and password are stored
 * here, sealed, and never rendered back. A save carries only the boxes typed,
 * never a value already stored.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { useDeleteRiderZwiftCredentials, useSetRiderZwiftCredentials } from "../../api/generated";
import { riderProfileQuery } from "../../api/queries";
import { Button } from "../../components/Button";
import { Skeleton } from "../../components/ui/skeleton";

function CardShell({ children }: { children: React.ReactNode }) {
  return (
    <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
      <CardHeader>
        <CardTitle role="heading" aria-level={2}>
          Zwift account
        </CardTitle>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
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
        className="grid gap-6"
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          onSave();
        }}
      >
        <FieldDescription>
          Zwift's API is unofficial and not supported by Zwift; your own account's terms of service
          still apply to reading it this way.
        </FieldDescription>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor={`${id}-email`}>Zwift email</FieldLabel>
            <Input
              id={`${id}-email`}
              type="password"
              autoComplete="off"
              placeholder={data.zwift.emailSet ? "Set — enter a new email to replace it" : ""}
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-password`}>Zwift password</FieldLabel>
            <Input
              id={`${id}-password`}
              type="password"
              autoComplete="off"
              placeholder={data.zwift.passwordSet ? "Set — enter a new password to replace it" : ""}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </Field>
        </FieldGroup>
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="default"
            aria-label="Save Zwift account"
            disabled={save.isPending || (email === "" && password === "")}
            onClick={onSave}
          >
            {save.isPending ? <Spinner aria-label="Saving" /> : null}
            Save
          </Button>
          <Button
            variant="outline"
            aria-label="Disconnect Zwift account"
            disabled={disconnect.isPending || (!data.zwift.emailSet && !data.zwift.passwordSet)}
            onClick={() => disconnect.mutate()}
          >
            {disconnect.isPending ? <Spinner aria-label="Disconnecting" /> : null}
            Disconnect
          </Button>
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
        </div>
      </form>
    </CardShell>
  );
}
