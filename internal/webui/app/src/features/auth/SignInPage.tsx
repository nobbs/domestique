/**
 * The one page a reader reaches before the gate admits them.
 *
 * The control is a document form rather than a fetch: what answers it is a
 * redirect out to the identity provider, which a page cannot follow itself.
 */

import { useSearchParams } from "react-router";
import { Button } from "@/components/Button";
import { Wordmark } from "@/components/brand/Wordmark";

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
    <main className="grid min-h-dvh place-items-center bg-[var(--base)] p-6">
      <div className="flex w-full max-w-xs flex-col items-center gap-6 rounded-xl border border-[var(--rule)] bg-[var(--panel)] p-8 shadow-[var(--shadow)]">
        <Wordmark />
        {message === null ? null : (
          // Assertive rather than polite: the reader arrived at this page for
          // the message, so it is the answer and not an aside.
          <p className="text-center text-sm text-[var(--alert)]" role="alert">
            {message}
          </p>
        )}
        <form action="/auth/start" className="w-full" method="post">
          <Button className="w-full" type="submit" variant="default">
            Sign in
          </Button>
        </form>
      </div>
    </main>
  );
}
