// Package config loads and validates Domestique's startup-only configuration.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/nobbs/domestique/internal/runtimeconfig"
)

const (
	defaultConfigFile = "/etc/domestique/config.toml"

	// defaultReadinessAddress is the readiness listener without configuration. A
	// second port rather than a second path, so a host publishing only the served
	// one cannot reach readiness through the front door.
	defaultReadinessAddress = ":8081"
	envPrefix               = "DOMESTIQUE_"
	configFileEnv           = envPrefix + "CONFIG_FILE"
	// The image the deploying host pinned. Named for what the host already calls
	// it, so the compose passthrough needs no translation table.
	imageReferenceEnv = envPrefix + "IMAGE"
)

// Settings is the validated, startup-only configuration for one service process:
// the two listeners, the one identity allowed to reach them, and where durable
// state lives with the key that encrypts it. Everything else is a runtime setting
// held in that state; this package refuses a file carrying one, naming the key.
type Settings struct {
	// ImageReference is not configuration: it is the image the deploying host
	// pinned, passed in so the service can report which image it is. It arrives
	// here because this package refuses unknown DOMESTIQUE_ keys. Empty when unset.
	ImageReference string

	HTTP   HTTP
	Access Access
	State  State
}

// HTTP configures the service listeners and the origin a browser reaches the
// served one at.
type HTTP struct {
	ListenAddress string

	// ReadinessAddress is the second listener answering the readiness probe. The
	// Tailnet host publishes and serves ListenAddress; only a local health check
	// reaches this one, so the two must never share a port.
	ReadinessAddress string

	// BrowserOriginURL is the public origin a browser reaches this service at. A
	// state-changing request naming any other is refused, and the Wahoo callback
	// is this origin's own path. Startup configuration because it is the gate: a
	// value the browser could edit is a gate the browser could open.
	BrowserOriginURL string
}

// Access identifies the sole user allowed to reach the service.
type Access struct {
	// Cloudflare configures the only gate the service has, and is required.
	Cloudflare CloudflareAccess
}

// CloudflareAccess configures verification of Cloudflare Access assertions. None
// of its values is a secret: the team domain and audience tag are public, and
// verification rests on Cloudflare's published signing keys.
type CloudflareAccess struct {
	// TeamDomain is the Zero Trust team domain that signs assertions.
	TeamDomain string

	// ApplicationAUD is the audience tag of the one Access application fronting
	// this service. Without it, an assertion for any other application of the
	// same team would verify.
	ApplicationAUD string

	// AllowedEmail is the single address an assertion may name.
	AllowedEmail string
}

// State configures durable service state.
type State struct {
	DatabasePath  string
	encryptionKey [32]byte
}

// EncryptionKey returns a copy of the key used to encrypt dynamic state.
func (s State) EncryptionKey() [32]byte {
	return s.encryptionKey
}

type rawSettings struct {
	HTTP   rawHTTP   `koanf:"http"`
	Access rawAccess `koanf:"access"`
	State  rawState  `koanf:"state"`
}

type rawHTTP struct {
	ListenAddress    string `koanf:"listen_address"`
	ReadinessAddress string `koanf:"readiness_address"`
	BrowserOriginURL string `koanf:"browser_origin_url"`
}

type rawAccess struct {
	Cloudflare rawCloudflareAccess `koanf:"cloudflare"`
}

type rawCloudflareAccess struct {
	TeamDomain     string `koanf:"team_domain"`
	ApplicationAUD string `koanf:"application_aud"`
	AllowedEmail   string `koanf:"allowed_email"`
}

type rawState struct {
	DatabasePath      string `koanf:"database_path"`
	EncryptionKeyFile string `koanf:"encryption_key_file"`
	EncryptionKey     string `koanf:"encryption_key"`
}

type secretInput struct {
	name      string
	directEnv string
	fileEnv   string
	filePath  string
}

// Load reads the configured TOML file and supported environment overrides once.
// It clears direct secret environment values before returning, including on
// validation failure.
func Load() (settings *Settings, err error) {
	defer func() {
		if clearErr := clearDirectSecretEnvironments(); clearErr != nil {
			settings = nil
			if err == nil {
				err = clearErr
			} else {
				err = errors.Join(err, clearErr)
			}
		}
	}()

	path, err := configuredPath()
	if err != nil {
		return nil, err
	}

	// Consumed before Koanf reads the environment, as the configuration selector
	// is: every remaining DOMESTIQUE_ variable is a setting, and unknown is fatal.
	imageReference, referenceErr := consumeImageReference()
	if referenceErr != nil {
		return nil, referenceErr
	}

	//nolint:gosec // The trusted operator selects the sole startup configuration file.
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading configuration: %w", err)
	}

	parser := toml.Parser()
	fileValues, err := parser.Unmarshal(contents)
	if err != nil {
		return nil, fmt.Errorf("parsing configuration: %w", err)
	}
	if err := rejectLiteralSecrets(fileValues); err != nil {
		return nil, err
	}

	k := koanf.New(".")
	if err := k.Load(confmap.Provider(configurationDefaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("loading defaults: %w", err)
	}
	if err := k.Load(file.Provider(path), parser); err != nil {
		return nil, fmt.Errorf("loading configuration: %w", err)
	}
	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: envPrefix,
		TransformFunc: func(key, value string) (string, any) {
			trimmed := strings.TrimPrefix(key, envPrefix)
			return strings.ToLower(strings.ReplaceAll(trimmed, "__", ".")), value
		},
	}), nil); err != nil {
		return nil, fmt.Errorf("loading environment: %w", err)
	}

	var raw rawSettings
	if err := k.UnmarshalWithConf("", &raw, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			ErrorUnused:      true,
			WeaklyTypedInput: false,
		},
	}); err != nil {
		return nil, fmt.Errorf("decoding configuration: %w", err)
	}

	built, buildErr := build(&raw)
	if buildErr != nil {
		return nil, buildErr
	}
	built.ImageReference = imageReference

	return built, nil
}

// consumeImageReference takes the pinned image reference out of the environment.
// Not validated here: the only consumer keeps just the digest it can prove.
func consumeImageReference() (string, error) {
	reference, found := os.LookupEnv(imageReferenceEnv)
	if !found {
		return "", nil
	}
	if err := os.Unsetenv(imageReferenceEnv); err != nil {
		return "", fmt.Errorf("clearing image reference: %w", err)
	}

	return reference, nil
}

func configuredPath() (string, error) {
	path, found := os.LookupEnv(configFileEnv)
	if found {
		if err := os.Unsetenv(configFileEnv); err != nil {
			return "", fmt.Errorf("clearing configuration selector: %w", err)
		}
		if strings.TrimSpace(path) == "" {
			return "", errors.New("configuration selector is empty")
		}
		return path, nil
	}

	return defaultConfigFile, nil
}

func rejectLiteralSecrets(values map[string]any) error {
	for _, path := range secretLiteralPaths() {
		if hasPath(values, path) {
			return fmt.Errorf("literal secret %q is not allowed in TOML", strings.Join(path, "."))
		}
	}

	return nil
}

func hasPath(values map[string]any, path []string) bool {
	current := values
	for index, segment := range path {
		value, found := current[segment]
		if !found {
			return false
		}
		if index == len(path)-1 {
			return true
		}

		next, ok := value.(map[string]any)
		if !ok {
			return false
		}
		current = next
	}

	return false
}

// validateCloudflareAccess accepts the section wholly absent or wholly present.
// A half-configured section would produce a public endpoint whose assertions
// are never checked.
func validateCloudflareAccess(raw *rawCloudflareAccess) error {
	values := map[string]string{
		"access.cloudflare.team_domain":     strings.TrimSpace(raw.TeamDomain),
		"access.cloudflare.application_aud": strings.TrimSpace(raw.ApplicationAUD),
		"access.cloudflare.allowed_email":   strings.TrimSpace(raw.AllowedEmail),
	}

	missing := make([]string, 0, len(values))
	for key, value := range values {
		if value == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)

	return fmt.Errorf("access.cloudflare is required; missing %s", strings.Join(missing, ", "))
}

func build(raw *rawSettings) (*Settings, error) {
	if err := validateListenAddress(raw.HTTP.ListenAddress); err != nil {
		return nil, err
	}
	if err := validateReadinessAddress(raw.HTTP.ReadinessAddress, raw.HTTP.ListenAddress); err != nil {
		return nil, err
	}
	browserOrigin := strings.TrimSpace(raw.HTTP.BrowserOriginURL)
	if err := runtimeconfig.ValidateHTTPSOrigin("http.browser_origin_url", browserOrigin); err != nil {
		return nil, fmt.Errorf("configuration file: %w", err)
	}
	if err := validateCloudflareAccess(&raw.Access.Cloudflare); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(raw.State.DatabasePath) {
		return nil, errors.New("state.database_path must be an absolute path")
	}

	keySecret, err := resolveSecret(secretInput{
		name:      "state encryption key",
		directEnv: envPrefix + "STATE__ENCRYPTION_KEY",
		fileEnv:   envPrefix + "STATE__ENCRYPTION_KEY_FILE",
		filePath:  raw.State.EncryptionKeyFile,
	})
	if err != nil {
		return nil, err
	}
	key, err := decodeKey(keySecret)
	if err != nil {
		return nil, err
	}

	return &Settings{
		HTTP: HTTP{
			ListenAddress:    raw.HTTP.ListenAddress,
			ReadinessAddress: strings.TrimSpace(raw.HTTP.ReadinessAddress),
			BrowserOriginURL: strings.TrimSuffix(browserOrigin, "/"),
		},
		Access: Access{
			Cloudflare: CloudflareAccess{
				TeamDomain:     strings.TrimSpace(raw.Access.Cloudflare.TeamDomain),
				ApplicationAUD: strings.TrimSpace(raw.Access.Cloudflare.ApplicationAUD),
				AllowedEmail:   strings.TrimSpace(raw.Access.Cloudflare.AllowedEmail),
			},
		},
		State: State{
			DatabasePath:  raw.State.DatabasePath,
			encryptionKey: key,
		},
	}, nil
}

func validateListenAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "" {
		return errors.New("http.listen_address must be a loopback-proxy listener such as :8080")
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return errors.New("http.listen_address must contain a valid port")
	}

	return nil
}

// validateReadinessAddress accepts a readiness listener on the served listener's
// terms and additionally refuses its port. Sharing it would put readiness behind
// Tailscale Serve and the tunnel.
func validateReadinessAddress(address, listenAddress string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || host != "" {
		return errors.New("http.readiness_address must be a loopback-proxy listener such as :8081")
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return errors.New("http.readiness_address must contain a valid port")
	}
	if _, listenPort, listenErr := net.SplitHostPort(listenAddress); listenErr == nil && listenPort == port {
		return errors.New("http.readiness_address must not be http.listen_address")
	}

	return nil
}

func resolveSecret(input secretInput) (runtimeconfig.Secret, error) {
	directValue, directSet := os.LookupEnv(input.directEnv)
	_, fileSet := os.LookupEnv(input.fileEnv)
	if directSet {
		if err := os.Unsetenv(input.directEnv); err != nil {
			return runtimeconfig.Secret{}, fmt.Errorf("clearing %s: %w", input.name, err)
		}
	}

	switch {
	case directSet && fileSet:
		return runtimeconfig.Secret{}, fmt.Errorf("%s has both direct and file environment inputs", input.name)
	case directSet && input.filePath != "":
		return runtimeconfig.Secret{}, fmt.Errorf("%s has both direct and file inputs", input.name)
	case directSet:
		return secretFromText(input.name, directValue)
	case input.filePath == "":
		return runtimeconfig.Secret{}, fmt.Errorf("%s is not configured", input.name)
	default:
		return secretFromFile(input.name, input.filePath)
	}
}

func secretFromFile(name, path string) (runtimeconfig.Secret, error) {
	if !filepath.IsAbs(path) {
		return runtimeconfig.Secret{}, fmt.Errorf("%s file must be an absolute path", name)
	}
	info, err := os.Stat(path)
	if err != nil {
		return runtimeconfig.Secret{}, fmt.Errorf("%s file is unavailable", name)
	}
	if !info.Mode().IsRegular() {
		return runtimeconfig.Secret{}, fmt.Errorf("%s file is not regular", name)
	}

	//nolint:gosec // The path is validated as an absolute, regular operator-managed secret file.
	contents, err := os.ReadFile(path)
	if err != nil {
		return runtimeconfig.Secret{}, fmt.Errorf("%s file is unreadable", name)
	}

	return secretFromText(name, trimTerminalLineBreak(string(contents)))
}

func secretFromText(name, value string) (runtimeconfig.Secret, error) {
	if value == "" {
		return runtimeconfig.Secret{}, fmt.Errorf("%s is empty", name)
	}

	return runtimeconfig.NewSecret([]byte(value)), nil
}

func trimTerminalLineBreak(value string) string {
	if trimmed, found := strings.CutSuffix(value, "\r\n"); found {
		return trimmed
	}

	trimmed, _ := strings.CutSuffix(value, "\n")
	return trimmed
}

func configurationDefaults() map[string]any {
	return map[string]any{
		"http": map[string]any{
			"readiness_address": defaultReadinessAddress,
		},
	}
}

func secretLiteralPaths() [][]string {
	return [][]string{
		{"state", "encryption_key"},
	}
}

func clearDirectSecretEnvironments() error {
	for _, name := range directSecretEnvironmentNames() {
		if err := os.Unsetenv(name); err != nil {
			return fmt.Errorf("clearing direct secret environment: %w", err)
		}
	}

	return nil
}

func directSecretEnvironmentNames() []string {
	return []string{
		envPrefix + "STATE__ENCRYPTION_KEY",
	}
}

func decodeKey(secret runtimeconfig.Secret) ([32]byte, error) {
	var key [32]byte
	encoded := string(secret.Bytes())
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return key, errors.New("state encryption key is not valid base64url")
		}
	}
	if len(decoded) != len(key) {
		return key, errors.New("state encryption key must decode to exactly 32 bytes")
	}
	copy(key[:], decoded)

	return key, nil
}
