package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/nobbs/domestique/internal/auth0"
)

const (
	// demoSubject is who the fake issuer authenticates. Also the one subject the
	// demo configuration allows.
	demoSubject = "demo|rider"

	// demoDisplay is what a real sign-in would settle on: the email claim.
	demoDisplay = "rider@example.test"

	// signingKeyBits matches the key size a real tenant signs with.
	signingKeyBits = 2048

	// idTokenLifetime is a working session rather than a day: an expired token
	// is a thing the flow has to keep handling correctly.
	idTokenLifetime = 8 * time.Hour

	certificateLifetime = 24 * time.Hour
)

// issuer is a local Auth0-shaped tenant: a signing key, the three endpoints the
// SDK talks to, and a self-signed certificate so the SDK's forced HTTPS reaches
// something. It serves on its own loopback port and can reach nothing else.
type issuer struct {
	private     *rsa.PrivateKey
	certificate tls.Certificate
	roots       *x509.CertPool
	address     string
	clientID    string
}

// newIssuer prepares a tenant on the given host:port. It listens only once
// serve is called.
func newIssuer(address, clientID string) (*issuer, error) {
	private, err := rsa.GenerateKey(rand.Reader, signingKeyBits)
	if err != nil {
		return nil, fmt.Errorf("generating a demo signing key: %w", err)
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("reading the demo issuer address: %w", err)
	}
	certificate, roots, err := selfSigned(private, host)
	if err != nil {
		return nil, err
	}

	return &issuer{
		private:     private,
		certificate: certificate,
		roots:       roots,
		address:     address,
		clientID:    clientID,
	}, nil
}

// serve starts the tenant's listener and returns a function that stops it.
func (i *issuer) serve() (stop func(), err error) {
	listener, err := tls.Listen("tcp", i.address, &tls.Config{
		Certificates: []tls.Certificate{i.certificate},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		return nil, fmt.Errorf("listening for the demo issuer: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /authorize", i.authorize)
	mux.HandleFunc("POST /oauth/token", i.token)
	mux.HandleFunc("GET /.well-known/jwks.json", i.keys)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: httpReadHeaderTimeout}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "demoapi: demo issuer: %v\n", serveErr)
		}
	}()

	return func() {
		if closeErr := server.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "demoapi: closing the demo issuer: %v\n", closeErr)
		}
	}, nil
}

// client is the production Auth0 adapter pointed at this tenant, trusting the
// certificate it just generated.
func (i *issuer) client(clientSecret, redirectURL string) (*auth0.Client, error) {
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: i.roots, MinVersion: tls.VersionTLS12}}
	client, err := auth0.New(&auth0.Options{
		Domain:       i.address,
		ClientID:     i.clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		HTTPClient:   &http.Client{Transport: transport, Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("configuring the demo sign-in provider: %w", err)
	}

	return client, nil
}

// authorize admits the caller at once: a demo has nobody to ask. The nonce
// rides back inside the code, so the token endpoint needs no state of its own.
func (i *issuer) authorize(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	redirect, err := url.Parse(query.Get("redirect_uri"))
	if err != nil || redirect.Host == "" {
		http.Error(writer, "redirect_uri is required", http.StatusBadRequest)

		return
	}
	back := redirect.Query()
	back.Set("state", query.Get("state"))
	back.Set("code", base64.RawURLEncoding.EncodeToString([]byte(query.Get("nonce"))))
	redirect.RawQuery = back.Encode()

	// A development-only issuer, on a loopback port, sending the caller back to
	// the redirect_uri it supplied.
	//nolint:gosec // G710: the target is the caller's own redirect_uri.
	http.Redirect(writer, request, redirect.String(), http.StatusFound)
}

func (i *issuer) token(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "unreadable form", http.StatusBadRequest)

		return
	}
	nonce, err := base64.RawURLEncoding.DecodeString(request.PostFormValue("code"))
	if err != nil {
		http.Error(writer, "unknown code", http.StatusBadRequest)

		return
	}
	idToken, err := i.mint(string(nonce))
	if err != nil {
		http.Error(writer, "cannot mint an id token", http.StatusInternalServerError)

		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"access_token": "demo-access-token",
		"id_token":     idToken,
		"token_type":   "Bearer",
		"expires_in":   int(idTokenLifetime.Seconds()),
	}); err != nil {
		return
	}
}

func (i *issuer) keys(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"keys": []map[string]string{{
			"kid": "demo",
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(i.private.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(i.private.E)).Bytes()),
		}},
	}); err != nil {
		return
	}
}

// mint signs one ID token. Stdlib rather than a JWT library: three base64url
// segments and one RSA signature is the whole of it.
func (i *issuer) mint(nonce string) (string, error) {
	now := time.Now()
	segments := []map[string]any{
		{"alg": "RS256", "kid": "demo", "typ": "JWT"},
		{
			// The SDK expects the issuer to carry a trailing slash.
			"iss":   "https://" + i.address + "/",
			"aud":   i.clientID,
			"sub":   demoSubject,
			"nonce": nonce,
			"email": demoDisplay,
			"name":  "Demo Rider",
			"iat":   now.Unix(),
			"exp":   now.Add(idTokenLifetime).Unix(),
		},
	}

	encoded := make([]string, 0, len(segments))
	for _, segment := range segments {
		raw, err := json.Marshal(segment)
		if err != nil {
			return "", fmt.Errorf("encoding an id token segment: %w", err)
		}
		encoded = append(encoded, base64.RawURLEncoding.EncodeToString(raw))
	}
	signingInput := encoded[0] + "." + encoded[1]
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, i.private, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing an id token: %w", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// selfSigned issues the certificate the tenant serves and the pool that trusts
// it, so the SDK's forced HTTPS has something to verify against.
func selfSigned(private *rsa.PrivateKey, host string) (tls.Certificate, *x509.CertPool, error) {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(certificateLifetime),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if address := net.ParseIP(host); address != nil {
		template.IPAddresses = []net.IP{address}
	} else {
		template.DNSNames = []string{host}
	}

	raw, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("issuing the demo certificate: %w", err)
	}
	parsed, err := x509.ParseCertificate(raw)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("reading the demo certificate: %w", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)

	return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: private, Leaf: parsed}, roots, nil
}

// signInProvider adapts the Auth0 client to session.Provider, which is kept to
// primitives so that package never imports this adapter.
type signInProvider struct{ client *auth0.Client }

func (p signInProvider) AuthorizationURL(ctx context.Context, state, nonce, codeVerifier string) (string, error) {
	return p.client.AuthorizationURL(ctx, state, nonce, codeVerifier) //nolint:wrapcheck // forwarding to the client this holds
}

func (p signInProvider) Exchange(ctx context.Context, code, codeVerifier, nonce string) (subject, email, name string, err error) {
	identity, err := p.client.Exchange(ctx, code, codeVerifier, nonce)
	if err != nil {
		return "", "", "", err //nolint:wrapcheck // forwarding to the client this holds
	}

	return identity.Subject, identity.Email, identity.Name, nil
}
