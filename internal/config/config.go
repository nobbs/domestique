// Package config loads and validates Domestique's static runtime settings.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	defaultConfigFile = "/etc/domestique/config.toml"
	envPrefix         = "DOMESTIQUE_"
	configFileEnv     = envPrefix + "CONFIG_FILE"

	// defaultTileStyleURL is a keyless MapLibre style, so the default deployment
	// exposes no credential to the browser and sends no account identity to the
	// tile origin.
	defaultTileStyleURL = "https://tiles.openfreemap.org/styles/bright"
)

// Settings is the validated, startup-only configuration for one service
// process. Its sensitive values are held in dedicated types without JSON tags.
type Settings struct {
	Wahoo         Wahoo
	VeloPlanner   VeloPlanner
	Notifications Notifications
	HTTP          HTTP
	Access        Access
	WebUI         WebUI
	Sync          Sync
	State         State
}

// HTTP configures the service listener.
type HTTP struct {
	ListenAddress string
}

// Access identifies the sole Tailnet user allowed to access the service.
type Access struct {
	TailnetUserLogin string
}

// WebUI configures the read-only browser route map view.
type WebUI struct {
	// TileStyleURL is the MapLibre style document the operator's browser loads.
	// It is deliberately not a secret: it is served to the page and is visible
	// to anyone who can reach the UI. The default is a keyless provider, so no
	// credential is exposed. A provider that requires an API key would place it
	// in this URL's query and thereby publish it to the browser.
	TileStyleURL string
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

// VeloPlanner configures the source account.
type VeloPlanner struct {
	BaseURL  string
	email    Secret
	password Secret
}

// Email returns the VeloPlanner account email as a dedicated secret value.
func (v VeloPlanner) Email() Secret {
	return v.email
}

// Password returns the VeloPlanner account password as a dedicated secret value.
func (v VeloPlanner) Password() Secret {
	return v.password
}

// Wahoo configures the OAuth application and destination target slots.
type Wahoo struct {
	APIBaseURL   string
	OAuthBaseURL string
	ClientID     string
	RedirectURL  string
	targets      []Target
	clientSecret Secret
}

// Targets returns a copy of the configured Wahoo target slots.
func (w *Wahoo) Targets() []Target {
	return append([]Target(nil), w.targets...)
}

// ClientSecret returns the OAuth client secret as a dedicated secret value.
func (w *Wahoo) ClientSecret() Secret {
	return w.clientSecret
}

// Target is one stable Wahoo destination slot.
type Target struct {
	ID string
}

// Sync configures the automatic reconciliation cadence and deletion gates.
type Sync struct {
	EmptySourceDeletion   EmptySourceDeletion
	InitialDelay          time.Duration
	Interval              time.Duration
	MaxDeletionsPerTarget int
}

// EmptySourceDeletion controls whether a trusted empty source can delete the
// final owned destination routes.
type EmptySourceDeletion string

const (
	// EmptySourceDeletionDeny blocks final-library deletion unless an operator
	// deliberately changes static configuration.
	EmptySourceDeletionDeny EmptySourceDeletion = "deny"
	// EmptySourceDeletionAllow permits final-library deletion while retaining
	// every other deletion safety gate.
	EmptySourceDeletionAllow EmptySourceDeletion = "allow"
)

// Notifications configures supported notification destinations.
type Notifications struct {
	Pushover Pushover
}

// Pushover holds the credentials needed to send a notification.
type Pushover struct {
	applicationToken Secret
	userKey          Secret
}

// ApplicationToken returns the Pushover application token as a secret value.
func (p Pushover) ApplicationToken() Secret {
	return p.applicationToken
}

// UserKey returns the Pushover recipient key as a secret value.
func (p Pushover) UserKey() Secret {
	return p.userKey
}

// Secret carries sensitive bytes without exposing them through formatting or
// JSON serialization.
type Secret struct {
	value []byte
}

// Bytes returns a defensive copy of the secret bytes.
func (s Secret) Bytes() []byte {
	return slices.Clone(s.value)
}

type rawSettings struct {
	Wahoo         rawWahoo         `koanf:"wahoo"`
	VeloPlanner   rawVeloPlanner   `koanf:"veloplanner"`
	Notifications rawNotifications `koanf:"notifications"`
	HTTP          rawHTTP          `koanf:"http"`
	Access        rawAccess        `koanf:"access"`
	WebUI         rawWebUI         `koanf:"webui"`
	State         rawState         `koanf:"state"`
	Sync          rawSync          `koanf:"sync"`
}

type rawHTTP struct {
	ListenAddress string `koanf:"listen_address"`
}

type rawWebUI struct {
	TileStyleURL string `koanf:"tile_style_url"`
}

type rawAccess struct {
	TailnetUserLogin string `koanf:"tailnet_user_login"`
}

type rawState struct {
	DatabasePath      string `koanf:"database_path"`
	EncryptionKeyFile string `koanf:"encryption_key_file"`
	EncryptionKey     string `koanf:"encryption_key"`
}

type rawVeloPlanner struct {
	BaseURL      string `koanf:"base_url"`
	EmailFile    string `koanf:"email_file"`
	Email        string `koanf:"email"`
	PasswordFile string `koanf:"password_file"`
	Password     string `koanf:"password"`
}

type rawWahoo struct {
	APIBaseURL       string      `koanf:"api_base_url"`
	OAuthBaseURL     string      `koanf:"oauth_base_url"`
	ClientID         string      `koanf:"client_id"`
	ClientSecretFile string      `koanf:"client_secret_file"`
	ClientSecret     string      `koanf:"client_secret"`
	RedirectURL      string      `koanf:"redirect_url"`
	Targets          []rawTarget `koanf:"targets"`
}

type rawTarget struct {
	ID string `koanf:"id"`
}

type rawSync struct {
	EmptySourceDeletion   string        `koanf:"empty_source_deletion"`
	InitialDelay          time.Duration `koanf:"initial_delay"`
	Interval              time.Duration `koanf:"interval"`
	MaxDeletionsPerTarget int           `koanf:"max_deletions_per_target"`
}

type rawNotifications struct {
	Pushover rawPushover `koanf:"pushover"`
}

type rawPushover struct {
	ApplicationTokenFile string `koanf:"application_token_file"`
	ApplicationToken     string `koanf:"application_token"`
	UserKeyFile          string `koanf:"user_key_file"`
	UserKey              string `koanf:"user_key"`
}

type secretInput struct {
	name      string
	directEnv string
	fileEnv   string
	filePath  string
}

// Load reads the configured TOML file and supported environment overrides once.
// It clears direct secret environment values before returning, including when
// validation fails.
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
			DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
			ErrorUnused:      true,
			WeaklyTypedInput: false,
		},
	}); err != nil {
		return nil, fmt.Errorf("decoding configuration: %w", err)
	}

	return build(&raw)
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

func build(raw *rawSettings) (*Settings, error) {
	if err := validateListenAddress(raw.HTTP.ListenAddress); err != nil {
		return nil, err
	}
	if err := requireValue("access.tailnet_user_login", raw.Access.TailnetUserLogin); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(raw.State.DatabasePath) {
		return nil, errors.New("state.database_path must be an absolute path")
	}
	if err := validateHTTPSURL("veloplanner.base_url", raw.VeloPlanner.BaseURL); err != nil {
		return nil, err
	}
	if err := validateHTTPSURL("wahoo.api_base_url", raw.Wahoo.APIBaseURL); err != nil {
		return nil, err
	}
	if err := validateHTTPSURL("wahoo.oauth_base_url", raw.Wahoo.OAuthBaseURL); err != nil {
		return nil, err
	}
	if err := requireValue("wahoo.client_id", raw.Wahoo.ClientID); err != nil {
		return nil, err
	}
	if err := validateRedirectURL(raw.Wahoo.RedirectURL); err != nil {
		return nil, err
	}
	if err := validateTileStyleURL(raw.WebUI.TileStyleURL); err != nil {
		return nil, err
	}

	targets, err := validateTargets(raw.Wahoo.Targets)
	if err != nil {
		return nil, err
	}
	err = validateSync(raw.Sync)
	if err != nil {
		return nil, err
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

	email, err := resolveSecret(secretInput{
		name:      "VeloPlanner email",
		directEnv: envPrefix + "VELOPLANNER__EMAIL",
		fileEnv:   envPrefix + "VELOPLANNER__EMAIL_FILE",
		filePath:  raw.VeloPlanner.EmailFile,
	})
	if err != nil {
		return nil, err
	}
	password, err := resolveSecret(secretInput{
		name:      "VeloPlanner password",
		directEnv: envPrefix + "VELOPLANNER__PASSWORD",
		fileEnv:   envPrefix + "VELOPLANNER__PASSWORD_FILE",
		filePath:  raw.VeloPlanner.PasswordFile,
	})
	if err != nil {
		return nil, err
	}
	clientSecret, err := resolveSecret(secretInput{
		name:      "Wahoo client secret",
		directEnv: envPrefix + "WAHOO__CLIENT_SECRET",
		fileEnv:   envPrefix + "WAHOO__CLIENT_SECRET_FILE",
		filePath:  raw.Wahoo.ClientSecretFile,
	})
	if err != nil {
		return nil, err
	}
	applicationToken, err := resolveSecret(secretInput{
		name:      "Pushover application token",
		directEnv: envPrefix + "NOTIFICATIONS__PUSHOVER__APPLICATION_TOKEN",
		fileEnv:   envPrefix + "NOTIFICATIONS__PUSHOVER__APPLICATION_TOKEN_FILE",
		filePath:  raw.Notifications.Pushover.ApplicationTokenFile,
	})
	if err != nil {
		return nil, err
	}
	userKey, err := resolveSecret(secretInput{
		name:      "Pushover user key",
		directEnv: envPrefix + "NOTIFICATIONS__PUSHOVER__USER_KEY",
		fileEnv:   envPrefix + "NOTIFICATIONS__PUSHOVER__USER_KEY_FILE",
		filePath:  raw.Notifications.Pushover.UserKeyFile,
	})
	if err != nil {
		return nil, err
	}

	return &Settings{
		HTTP: HTTP{
			ListenAddress: raw.HTTP.ListenAddress,
		},
		Access: Access{
			TailnetUserLogin: strings.TrimSpace(raw.Access.TailnetUserLogin),
		},
		WebUI: WebUI{
			TileStyleURL: strings.TrimSpace(raw.WebUI.TileStyleURL),
		},
		State: State{
			DatabasePath:  raw.State.DatabasePath,
			encryptionKey: key,
		},
		VeloPlanner: VeloPlanner{
			BaseURL:  raw.VeloPlanner.BaseURL,
			email:    email,
			password: password,
		},
		Wahoo: Wahoo{
			APIBaseURL:   raw.Wahoo.APIBaseURL,
			OAuthBaseURL: raw.Wahoo.OAuthBaseURL,
			ClientID:     strings.TrimSpace(raw.Wahoo.ClientID),
			RedirectURL:  raw.Wahoo.RedirectURL,
			targets:      targets,
			clientSecret: clientSecret,
		},
		Sync: Sync{
			InitialDelay:          raw.Sync.InitialDelay,
			Interval:              raw.Sync.Interval,
			MaxDeletionsPerTarget: raw.Sync.MaxDeletionsPerTarget,
			EmptySourceDeletion:   EmptySourceDeletion(raw.Sync.EmptySourceDeletion),
		},
		Notifications: Notifications{
			Pushover: Pushover{
				applicationToken: applicationToken,
				userKey:          userKey,
			},
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

// validateTileStyleURL accepts an absolute HTTPS style document URL. Unlike the
// service's own endpoints it permits a query string, because providers that
// require an API key carry it there; such a key is published to the browser and
// is the operator's deliberate choice, not a managed secret.
func validateTileStyleURL(value string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return errors.New("webui.tile_style_url must be an absolute HTTPS URL without credentials or fragment")
	}

	return nil
}

func validateHTTPSURL(name, value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute HTTPS URL without credentials, query, or fragment", name)
	}

	return nil
}

func validateRedirectURL(value string) error {
	if err := validateHTTPSURL("wahoo.redirect_url", value); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return errors.New("wahoo.redirect_url must be a valid URL")
	}
	if parsed.Path != "/oauth/wahoo/callback" {
		return errors.New("wahoo.redirect_url must end in /oauth/wahoo/callback")
	}

	return nil
}

func validateTargets(raw []rawTarget) ([]Target, error) {
	if len(raw) < 1 || len(raw) > 2 {
		return nil, errors.New("wahoo.targets must contain between one and two entries")
	}

	targets := make([]Target, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for index, target := range raw {
		id := strings.TrimSpace(target.ID)
		if id == "" {
			return nil, fmt.Errorf("wahoo.targets[%d].id is required", index)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("wahoo.targets[%d].id is duplicated", index)
		}
		seen[id] = struct{}{}
		targets[index] = Target{ID: id}
	}

	return targets, nil
}

func validateSync(sync rawSync) error {
	if sync.InitialDelay <= 0 {
		return errors.New("sync.initial_delay must be positive")
	}
	if sync.Interval != time.Hour {
		return errors.New("sync.interval must equal 1h")
	}
	if sync.MaxDeletionsPerTarget != 5 {
		return errors.New("sync.max_deletions_per_target must equal 5")
	}
	if sync.EmptySourceDeletion != string(EmptySourceDeletionDeny) &&
		sync.EmptySourceDeletion != string(EmptySourceDeletionAllow) {
		return errors.New("sync.empty_source_deletion must be deny or allow")
	}

	return nil
}

func requireValue(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}

	return nil
}

func resolveSecret(input secretInput) (Secret, error) {
	directValue, directSet := os.LookupEnv(input.directEnv)
	_, fileSet := os.LookupEnv(input.fileEnv)
	if directSet {
		if err := os.Unsetenv(input.directEnv); err != nil {
			return Secret{}, fmt.Errorf("clearing %s: %w", input.name, err)
		}
	}

	switch {
	case directSet && fileSet:
		return Secret{}, fmt.Errorf("%s has both direct and file environment inputs", input.name)
	case directSet && input.filePath != "":
		return Secret{}, fmt.Errorf("%s has both direct and file inputs", input.name)
	case directSet:
		return secretFromText(input.name, directValue)
	case input.filePath == "":
		return Secret{}, fmt.Errorf("%s is not configured", input.name)
	default:
		return secretFromFile(input.name, input.filePath)
	}
}

func secretFromFile(name, path string) (Secret, error) {
	if !filepath.IsAbs(path) {
		return Secret{}, fmt.Errorf("%s file must be an absolute path", name)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Secret{}, fmt.Errorf("%s file is unavailable", name)
	}
	if !info.Mode().IsRegular() {
		return Secret{}, fmt.Errorf("%s file is not regular", name)
	}

	//nolint:gosec // The path is validated as an absolute, regular operator-managed secret file.
	contents, err := os.ReadFile(path)
	if err != nil {
		return Secret{}, fmt.Errorf("%s file is unreadable", name)
	}

	return secretFromText(name, trimTerminalLineBreak(string(contents)))
}

func secretFromText(name, value string) (Secret, error) {
	if value == "" {
		return Secret{}, fmt.Errorf("%s is empty", name)
	}

	return Secret{value: []byte(value)}, nil
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
		"sync": map[string]any{
			"empty_source_deletion": string(EmptySourceDeletionDeny),
		},
		"webui": map[string]any{
			"tile_style_url": defaultTileStyleURL,
		},
	}
}

func secretLiteralPaths() [][]string {
	return [][]string{
		{"state", "encryption_key"},
		{"veloplanner", "email"},
		{"veloplanner", "password"},
		{"wahoo", "client_secret"},
		{"notifications", "pushover", "application_token"},
		{"notifications", "pushover", "user_key"},
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
		envPrefix + "VELOPLANNER__EMAIL",
		envPrefix + "VELOPLANNER__PASSWORD",
		envPrefix + "WAHOO__CLIENT_SECRET",
		envPrefix + "NOTIFICATIONS__PUSHOVER__APPLICATION_TOKEN",
		envPrefix + "NOTIFICATIONS__PUSHOVER__USER_KEY",
	}
}

func decodeKey(secret Secret) ([32]byte, error) {
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
