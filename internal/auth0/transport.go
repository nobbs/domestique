package auth0

import (
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
		if bounded.Timeout <= 0 {
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
	response.Body = limitedBody{
		Reader: io.LimitReader(response.Body, maxResponseBytes),
		Closer: response.Body,
	}

	return response, nil
}

type limitedBody struct {
	io.Reader
	io.Closer
}
