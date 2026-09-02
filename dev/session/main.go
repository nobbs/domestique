// Command session mints a browser session directly into a state database, so
// development and the demo can serve a gated UI without reaching a sign-in
// provider.
//
// Development tooling, not part of the shipped binary. See dev/setup.sh.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nobbs/domestique/internal/sqlite"
)

// lifetime matches internal/session's own, so a minted row expires the way one
// created by a real sign-in does.
const lifetime = 24 * time.Hour

func main() {
	database := flag.String("database", "", "state database to mint the session into")
	subject := flag.String("subject", "", "subject claim the session belongs to")
	admin := flag.Bool("admin", false, "mint the session with cross-subject rights")
	out := flag.String("out", "", "write the raw token here instead of to stdout")
	flag.Parse()

	if err := run(*database, *subject, *out, *admin); err != nil {
		fmt.Fprintf(os.Stderr, "session: %v\n", err)
		os.Exit(1)
	}
}

func run(database, subject, out string, admin bool) error {
	if database == "" || subject == "" {
		return errors.New("-database and -subject are required")
	}

	ctx := context.Background()
	// A placeholder key: this tool writes no encrypted column, and the
	// development snapshot's stored credentials are already undecryptable.
	var key [32]byte
	store, err := sqlite.Open(ctx, database, key)
	if err != nil {
		return fmt.Errorf("opening state: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "session: closing state: %v\n", closeErr)
		}
	}()

	token, digest, err := mint()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := store.CreateSession(ctx, digest, subject, subject, admin, now, now.Add(lifetime)); err != nil {
		return fmt.Errorf("storing session: %w", err)
	}

	if out == "" {
		fmt.Println(token)

		return nil
	}
	if err := os.WriteFile(out, []byte(token), 0o600); err != nil {
		return fmt.Errorf("writing token: %w", err)
	}

	return nil
}

// mint reproduces internal/session's token derivation: 32 random bytes as the
// wire value, their SHA-256 as what is stored.
func mint() (wire string, digest []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("reading randomness: %w", err)
	}
	sum := sha256.Sum256(raw)

	return base64.RawURLEncoding.EncodeToString(raw), sum[:], nil
}
