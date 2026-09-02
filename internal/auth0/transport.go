package auth0

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// defaultTimeout bounds both the token exchange and the SDK's own JWKS fetch.
const defaultTimeout = 5 * time.Second

// maxResponseBytes caps every response body: Auth0 replies are kilobytes, and a
// hostile or broken endpoint must not be buffered in full.
const maxResponseBytes = 1 << 20

// boundedHTTPClient bounds every outbound call and caps every response body,
// so a public callback endpoint cannot be made to buffer an unbounded reply.
// A caller-supplied client is copied rather than used as it stands: the cap is
// a guarantee of this package, not of whoever passed the client in.
func boundedHTTPClient(client *http.Client) *http.Client {
	bounded := &http.Client{Timeout: defaultTimeout, Transport: http.DefaultTransport}
	if client != nil {
		copied := *client
		bounded = &copied
		// A shorter timeout is a caller's to choose; a longer one is not, or the
		// public callback handler waits past the bound this package promises.
		if bounded.Timeout <= 0 || bounded.Timeout > defaultTimeout {
			bounded.Timeout = defaultTimeout
		}
		if bounded.Transport == nil {
			bounded.Transport = http.DefaultTransport
		}
	}
	if _, already := bounded.Transport.(*boundedTransport); !already {
		bounded.Transport = &boundedTransport{base: bounded.Transport}
	}

	return bounded
}

// boundedTransport wraps a RoundTripper to cap the response body it returns.
type boundedTransport struct {
	base http.RoundTripper
}

func (t *boundedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("auth0: request failed: %w", err)
	}
	response.Body = &limitedBody{body: response.Body, remaining: maxResponseBytes}

	return response, nil
}

// errResponseTooLarge is what a reader of an over-cap body sees. Truncating to
// a clean EOF instead would hand the caller a body that parses as a short but
// complete reply.
var errResponseTooLarge = errors.New("auth0: response exceeded size limit")

// limitedBody yields at most maxResponseBytes and then fails, reading one byte
// past the cap to tell a body that just fits from one that does not.
type limitedBody struct {
	body      io.ReadCloser
	remaining int64
	exceeded  bool
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.exceeded {
		return 0, errResponseTooLarge
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}

	read, err := b.body.Read(p)
	if int64(read) > b.remaining {
		b.exceeded = true

		return 0, errResponseTooLarge
	}
	b.remaining -= int64(read)

	return read, err //nolint:wrapcheck // the body's own read error belongs to its reader.
}

func (b *limitedBody) Close() error {
	return b.body.Close() //nolint:wrapcheck // closing is the wrapped body's own concern.
}
