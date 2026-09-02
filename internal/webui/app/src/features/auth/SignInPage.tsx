/**
 * The one page a reader reaches before the gate admits them.
 *
 * The control is a document form rather than a fetch: what answers it is a
 * redirect out to the identity provider, which a page cannot follow itself.
 */

import { useSearchParams } from "react-router";
import { Button } from "@/components/Button";
import { Wordmark } from "@/components/brand/Wordmark";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

/**
 * Why a sign-in came back here. A refused account is told apart from a failed
 * attempt so it is not read as an outage; which account is not carried,
 * because a query string outlives the answer it was part of.
 */
// A Map rather than an object: the key is whatever the address carried, and a
// plain object answers `__proto__` or `constructor` from its prototype.
const REFUSALS = new Map<string, string>([
  ["not_allowed", "This account is not allowed to sign in."],
]);

/** Nothing at all where nothing failed, and one sentence for every step that can. */
export function refusalMessage(error: string | null): string | null {
  if (error === null) {
    return null;
  }

  return REFUSALS.get(error) ?? "Sign-in could not be completed.";
}

export function SignInPage() {
  const [params] = useSearchParams();
  const message = refusalMessage(params.get("error"));

  return (
    <main className="flex min-h-svh flex-col items-center justify-center bg-[var(--base)] p-6 md:p-10">
      <Card className="w-full max-w-sm border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
        <CardHeader>
          <CardTitle className="flex justify-center">
            <Wordmark className="gap-3 text-2xl" size={40} />
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4">
          {message === null ? null : (
            <Alert variant="destructive" className="border-0 bg-transparent text-center">
              <AlertTitle>{message}</AlertTitle>
            </Alert>
          )}
          <form action="/auth/start" className="flex justify-center" method="post">
            <Button type="submit" variant="default">
              Sign in
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
