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

	"github.com/nobbs/domestique/internal/runtimeconfig"
)

const (
	defaultConfigFile = "/etc/domestique/config.toml"

	// defaultReadinessAddress is the readiness listener an operator gets without
	// saying anything. It is a second port rather than a second path, so a host
	// that publishes only the served one has no way to reach readiness through
	// the front door — and an existing deployment keeps working unchanged,
	// because nothing has to be added to its configuration file.
	defaultReadinessAddress = ":8081"
	envPrefix               = "DOMESTIQUE_"
	configFileEnv           = envPrefix + "CONFIG_FILE"
	// The image the deploying host pinned. Named for what the host already calls
	// it, so the compose passthrough needs no translation table.
	imageReferenceEnv = envPrefix + "IMAGE"
)

// Settings is the validated, startup-only configuration for one service
// process. Its sensitive values are held in dedicated types without JSON tags.
type Settings struct {
	// ImageReference is not configuration: it is the container image the
	// deploying host pinned, which the host passes in so the running service can
	// report which image it is. It arrives here only because this package owns
	// the DOMESTIQUE_ prefix and refuses unknown keys within it — leaving it in
	// the environment would fail startup. Empty when the host said nothing.
	ImageReference string

	Wahoo Wahoo
	// VeloPlanner and Komoot are nil when their section is not configured. At
	// least one is always non-nil in a Settings a caller actually receives:
	// build refuses a configuration naming neither.
	VeloPlanner   *VeloPlanner
	Komoot        *Komoot
	Notifications Notifications
	HTTP          HTTP
	Access        Access
	RideModel     RideModel
	State         State
	Sync          Sync
}

// HTTP configures the service listeners.
type HTTP struct {
	ListenAddress string

	// ReadinessAddress is the second, separate listener that answers the
	// readiness probe. It exists apart from ListenAddress because the two
	// answer different callers: the Tailnet host publishes and serves the first
	// one, and only a local health check reaches the second. Keeping readiness
	// off the served listener is what keeps it off the authenticated public
	// surface, so the two must never be the same port.
	ReadinessAddress string
}

// Access identifies the sole user allowed to reach the service.
type Access struct {
	// Cloudflare configures the only gate the service has, and is required.
	Cloudflare CloudflareAccess
}

// CloudflareAccess configures verification of Cloudflare Access assertions.
// None of its values is a secret: the team domain and the application audience
// tag are public identifiers, and verification rests on Cloudflare's published
// signing keys rather than on a shared credential.
type CloudflareAccess struct {
	// TeamDomain is the Zero Trust team domain that signs assertions.
	TeamDomain string

	// ApplicationAUD is the audience tag of the one Access application that
	// fronts this service. Without it, an assertion minted for any other
	// application of the same team would verify.
	ApplicationAUD string

	// AllowedEmail is the single address an assertion may name.
	AllowedEmail string
}

// RideModel configures the predicted moving time internal/ridemodel computes
// per stage.
type RideModel struct {
	// CoefficientsFile names the coefficient file #215 fits: mass, power, drag
	// area, rolling resistance per surface, and the descent constants. It is
	// deliberately not a secret — it carries no credential and no route data —
	// so it is a plain path like the OpenStreetMap extract configuration
	// rather than a *_file secret input.
	//
	// The default is no file, which switches prediction off entirely: no
	// coefficient is loaded, and no stage anywhere carries a predicted time.
	CoefficientsFile string
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

// Komoot configures the source account. Its shape coincides with
// VeloPlanner's today because both providers settled on the same
// email-and-password account-credential model; that coincidence is not a
// reason to share a type between them, since a future source's credentials
// need not look like either.
type Komoot struct {
	BaseURL  string
	email    Secret
	password Secret
}

// Email returns the Komoot account email as a dedicated secret value.
func (k Komoot) Email() Secret {
	return k.email
}

// Password returns the Komoot account password as a dedicated secret value.
func (k Komoot) Password() Secret {
	return k.password
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

// Sync configures the one part of the reconciliation cadence a restart is the
// only way to change: how long after start the first run is attempted. The
// deletion gate, the staleness bound and the success policy are runtime
// settings, held in the database and edited from the web UI.
type Sync struct {
	InitialDelay time.Duration
}

// Notifications holds the credentials a notification is sent with. How much
// reaches them, and the origin they are sent to, are runtime settings.
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
	VeloPlanner   rawVeloPlanner   `koanf:"veloplanner"`
	Komoot        rawKomoot        `koanf:"komoot"`
	Notifications rawNotifications `koanf:"notifications"`
	HTTP          rawHTTP          `koanf:"http"`
	Access        rawAccess        `koanf:"access"`
	RideModel     rawRideModel     `koanf:"ridemodel"`
	State         rawState         `koanf:"state"`
	Wahoo         rawWahoo         `koanf:"wahoo"`
	Sync          rawSync          `koanf:"sync"`
}

type rawHTTP struct {
	ListenAddress    string `koanf:"listen_address"`
	ReadinessAddress string `koanf:"readiness_address"`
}

type rawRideModel struct {
	CoefficientsFile string `koanf:"coefficients_file"`
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

type rawVeloPlanner struct {
	BaseURL      string `koanf:"base_url"`
	EmailFile    string `koanf:"email_file"`
	Email        string `koanf:"email"`
	PasswordFile string `koanf:"password_file"`
	Password     string `koanf:"password"`
}

type rawKomoot struct {
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
	InitialDelay time.Duration `koanf:"initial_delay"`
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

	// Consumed before Koanf reads the environment, for the same reason the
	// configuration selector is: every remaining DOMESTIQUE_ variable is treated
	// as a setting, and an unknown one is fatal.
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
			DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
			ErrorUnused:      true,
			WeaklyTypedInput: false,
		},
	}); err != nil {
		return nil, fmt.Errorf("decoding configuration: %w", err)
	}

	// A section's fields decode to their zero value whether the section was
	// written or not, so presence has to be asked of Koanf directly, before
	// build turns the raw zero values into "not configured".
	veloPlannerConfigured := k.Exists("veloplanner")
	komootConfigured := k.Exists("komoot")

	built, buildErr := build(&raw, veloPlannerConfigured, komootConfigured)
	if buildErr != nil {
		return nil, buildErr
	}
	built.ImageReference = imageReference

	return built, nil
}

// consumeImageReference takes the pinned image reference out of the environment
// and returns it. It is not validated here: this package cannot say what a valid
// image reference is, and the only consumer keeps just the digest it can prove.
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

// validateCloudflareAccess accepts the section either wholly absent or wholly
// present. A half-configured section is rejected rather than silently ignored,
// because the failure it would otherwise produce is a public endpoint whose
// assertions are never checked.
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

func build(raw *rawSettings, veloPlannerConfigured, komootConfigured bool) (*Settings, error) {
	if err := validateListenAddress(raw.HTTP.ListenAddress); err != nil {
		return nil, err
	}
	if err := validateReadinessAddress(raw.HTTP.ReadinessAddress, raw.HTTP.ListenAddress); err != nil {
		return nil, err
	}
	if err := validateCloudflareAccess(&raw.Access.Cloudflare); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(raw.State.DatabasePath) {
		return nil, errors.New("state.database_path must be an absolute path")
	}
	if !veloPlannerConfigured && !komootConfigured {
		return nil, errors.New("at least one source must be configured: add a [veloplanner] or [komoot] section")
	}
	if err := runtimeconfig.ValidateHTTPSURL("wahoo.api_base_url", raw.Wahoo.APIBaseURL); err != nil {
		return nil, fmt.Errorf("configuration file: %w", err)
	}
	if err := runtimeconfig.ValidateHTTPSURL("wahoo.oauth_base_url", raw.Wahoo.OAuthBaseURL); err != nil {
		return nil, fmt.Errorf("configuration file: %w", err)
	}
	if err := requireValue("wahoo.client_id", raw.Wahoo.ClientID); err != nil {
		return nil, err
	}
	if err := validateRedirectURL(raw.Wahoo.RedirectURL); err != nil {
		return nil, err
	}
	if err := validateRideModel(raw.RideModel); err != nil {
		return nil, err
	}

	targets, err := validateTargets(raw.Wahoo.Targets)
	if err != nil {
		return nil, err
	}
	if syncErr := validateSync(raw.Sync); syncErr != nil {
		return nil, syncErr
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

	var veloPlanner *VeloPlanner
	if veloPlannerConfigured {
		// An origin, not merely an absolute HTTPS URL: the adapter itself
		// requires no path (or exactly "/"), and rejecting the mismatch here
		// means a bad value fails at config load with a name and a reason,
		// rather than later at client construction with neither.
		if urlErr := runtimeconfig.ValidateHTTPSOrigin("veloplanner.base_url", raw.VeloPlanner.BaseURL); urlErr != nil {
			return nil, fmt.Errorf("configuration file: %w", urlErr)
		}
		email, emailErr := resolveSecret(secretInput{
			name:      "VeloPlanner email",
			directEnv: envPrefix + "VELOPLANNER__EMAIL",
			fileEnv:   envPrefix + "VELOPLANNER__EMAIL_FILE",
			filePath:  raw.VeloPlanner.EmailFile,
		})
		if emailErr != nil {
			return nil, emailErr
		}
		password, passwordErr := resolveSecret(secretInput{
			name:      "VeloPlanner password",
			directEnv: envPrefix + "VELOPLANNER__PASSWORD",
			fileEnv:   envPrefix + "VELOPLANNER__PASSWORD_FILE",
			filePath:  raw.VeloPlanner.PasswordFile,
		})
		if passwordErr != nil {
			return nil, passwordErr
		}
		veloPlanner = &VeloPlanner{BaseURL: raw.VeloPlanner.BaseURL, email: email, password: password}
	}

	var komoot *Komoot
	if komootConfigured {
		// Same reasoning as veloplanner.base_url above.
		if urlErr := runtimeconfig.ValidateHTTPSOrigin("komoot.base_url", raw.Komoot.BaseURL); urlErr != nil {
			return nil, fmt.Errorf("configuration file: %w", urlErr)
		}
		email, emailErr := resolveSecret(secretInput{
			name:      "Komoot email",
			directEnv: envPrefix + "KOMOOT__EMAIL",
			fileEnv:   envPrefix + "KOMOOT__EMAIL_FILE",
			filePath:  raw.Komoot.EmailFile,
		})
		if emailErr != nil {
			return nil, emailErr
		}
		password, passwordErr := resolveSecret(secretInput{
			name:      "Komoot password",
			directEnv: envPrefix + "KOMOOT__PASSWORD",
			fileEnv:   envPrefix + "KOMOOT__PASSWORD_FILE",
			filePath:  raw.Komoot.PasswordFile,
		})
		if passwordErr != nil {
			return nil, passwordErr
		}
		komoot = &Komoot{BaseURL: raw.Komoot.BaseURL, email: email, password: password}
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
			ListenAddress:    raw.HTTP.ListenAddress,
			ReadinessAddress: strings.TrimSpace(raw.HTTP.ReadinessAddress),
		},
		Access: Access{
			Cloudflare: CloudflareAccess{
				TeamDomain:     strings.TrimSpace(raw.Access.Cloudflare.TeamDomain),
				ApplicationAUD: strings.TrimSpace(raw.Access.Cloudflare.ApplicationAUD),
				AllowedEmail:   strings.TrimSpace(raw.Access.Cloudflare.AllowedEmail),
			},
		},
		RideModel: RideModel{
			CoefficientsFile: raw.RideModel.CoefficientsFile,
		},
		State: State{
			DatabasePath:  raw.State.DatabasePath,
			encryptionKey: key,
		},
		VeloPlanner: veloPlanner,
		Komoot:      komoot,
		Wahoo: Wahoo{
			APIBaseURL:   raw.Wahoo.APIBaseURL,
			OAuthBaseURL: raw.Wahoo.OAuthBaseURL,
			ClientID:     strings.TrimSpace(raw.Wahoo.ClientID),
			RedirectURL:  raw.Wahoo.RedirectURL,
			targets:      targets,
			clientSecret: clientSecret,
		},
		Sync: Sync{
			InitialDelay: raw.Sync.InitialDelay,
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

// validateReadinessAddress accepts a listener for the readiness probe on the
// same terms as the served one, and additionally refuses the served listener's
// own port.
// Sharing it would put readiness behind Tailscale Serve and the tunnel, which is
// exactly the surface the probe is supposed to stay off.
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

// validateRideModel accepts an unconfigured coefficients file — the operator's
// switch for leaving every stage without a predicted moving time — or an
// absolute path to one. The file itself is read, parsed, and validated for
// physical plausibility by internal/ridemodel at composition, not here: this
// package validates shape, the way it does for state.database_path, and
// leaves what the file means to the package that owns the model.
func validateRideModel(rideModel rawRideModel) error {
	if rideModel.CoefficientsFile == "" {
		return nil
	}
	if !filepath.IsAbs(rideModel.CoefficientsFile) {
		return errors.New("ridemodel.coefficients_file must be an absolute path")
	}

	return nil
}

func validateRedirectURL(value string) error {
	if err := runtimeconfig.ValidateHTTPSURL("wahoo.redirect_url", value); err != nil {
		return fmt.Errorf("configuration file: %w", err)
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
		"http": map[string]any{
			"readiness_address": defaultReadinessAddress,
		},
	}
}

func secretLiteralPaths() [][]string {
	return [][]string{
		{"state", "encryption_key"},
		{"veloplanner", "email"},
		{"veloplanner", "password"},
		{"komoot", "email"},
		{"komoot", "password"},
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
		envPrefix + "KOMOOT__EMAIL",
		envPrefix + "KOMOOT__PASSWORD",
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
