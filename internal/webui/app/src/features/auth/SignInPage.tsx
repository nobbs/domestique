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
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

/**
 * Why a sign-in came back here. A refused account is told apart from a failed
 * attempt so it is not read as an outage; which account is not carried,
 * because a query string outlives the answer it was part of.
 */
const REFUSALS: Record<string, string> = {
  not_allowed: "This account is not allowed to sign in.",
};

/** Nothing at all where nothing failed, and one sentence for every step that can. */
export function refusalMessage(error: string | null): string | null {
  if (error === null) {
    return null;
  }

  return REFUSALS[error] ?? "Sign-in could not be completed.";
}

export function SignInPage() {
  const [params] = useSearchParams();
  const message = refusalMessage(params.get("error"));

  return (
    <main className="flex min-h-svh flex-col items-center justify-center gap-6 bg-[var(--base)] p-6 md:p-10">
      <div className="flex w-full max-w-sm flex-col gap-6">
        <div className="self-center">
          <Wordmark />
        </div>
        <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
          <CardHeader className="text-center">
            <CardTitle className="text-xl">Welcome back</CardTitle>
            <CardDescription>Sign in with the configured account.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            {message === null ? null : (
              <Alert variant="destructive" className="border-[var(--rule)]">
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
      </div>
    </main>
  );
}
