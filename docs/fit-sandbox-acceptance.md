# FIT sandbox acceptance

`internal/fit/wahoo_sandbox_test.go` is the opt-in acceptance harness for the
FIT encoder. It is excluded from normal tests and CI. It creates a synthetic
two-point route in a **Wahoo sandbox** account, retrieves it, verifies the
remote route ID, external ID, and FIT file URL, then deletes the temporary
route during test cleanup.

It intentionally uses neither VeloPlanner data nor a configured service token.
Provide a short-lived sandbox access token only through the process environment:

```sh
DOMESTIQUE_WAHOO_ENVIRONMENT=sandbox \
DOMESTIQUE_WAHOO_SANDBOX_ACCESS_TOKEN=... \
mise exec -- go test -tags=wahoo_sandbox ./internal/fit
```

The API origin defaults to `https://api.wahooligan.com`. Set
`DOMESTIQUE_WAHOO_SANDBOX_BASE_URL` only when Wahoo supplies a different
sandbox origin. The test rejects non-HTTPS origins and refuses to run unless
the explicit environment value is `sandbox`.

Do not run the harness against a production account. The later Wahoo adapter
will supply the same direct-route operation to the service through encrypted
refresh-token state; this harness stays an independent manual check of encoder
compatibility.
