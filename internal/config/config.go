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
	"regexp"
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

	// defaultBasemapName labels the one basemap a deployment gets without
	// configuring any. It is never shown on its own — the picker appears only
	// where there is something to pick between — but it is still the entry's
	// identity, so it is a word an operator would recognise rather than a slug.
	defaultBasemapName = "Streets"

	// defaultBasemapStyleURL is a keyless MapLibre style, so the default
	// deployment exposes no credential to the browser and sends no account
	// identity to the tile origin.
	defaultBasemapStyleURL = "https://tiles.openfreemap.org/styles/bright"

	// defaultBasemapStyleURLDark is the same provider's dark style. It shares the
	// light default's origin, so following the operator's system colour scheme
	// costs a default deployment no second tile origin.
	defaultBasemapStyleURLDark = "https://tiles.openfreemap.org/styles/dark"

	// defaultRebuildInterval is how often the surface index is rebuilt when an
	// operator has named regions but not a cadence. OpenStreetMap extracts are
	// republished daily and a road's surface changes on the timescale of
	// resurfacing work, so a week is frequent enough to be current and rare
	// enough that the cost of a build never matters.
	defaultRebuildInterval = 7 * 24 * time.Hour

	// defaultPushoverURL is Pushover's own origin. It is a default rather than a
	// compiled-in constant so a development or demo environment can point it at
	// an address that goes nowhere instead of reaching the real service with a
	// placeholder token.
	defaultPushoverURL = "https://api.pushover.net"
	// defaultDigestInterval is the period a success digest covers when the
	// operator selects one without naming a period. A day is the cadence a
	// digest exists for: short enough to still describe a run, long enough that
	// it is not the per-run push it replaces.
	defaultDigestInterval = 24 * time.Hour
	// maxDigestInterval bounds the period one digest may cover.
	//
	// A digest totals the recorded run history, and that history is a bounded
	// window: the store keeps the last few hundred runs, which an hourly
	// deployment fills in a little over a week. A longer period would not fail —
	// it would quietly report a total missing every run already pruned from
	// under it, which is worse than refusing the setting.
	maxDigestInterval = 7 * 24 * time.Hour

	// defaultStaleAfter is how long the trusted source inventory may go without
	// a successful refresh before it is reported and notified as stale, when the
	// operator names no bound of their own. A day tolerates a bad run or two at
	// the hourly cadence while still catching a source that has quietly stopped
	// refreshing.
	defaultStaleAfter = 24 * time.Hour
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

	Wahoo         Wahoo
	VeloPlanner   VeloPlanner
	Notifications Notifications
	HTTP          HTTP
	Access        Access
	WebUI         WebUI
	Surface       Surface
	Sync          Sync
	State         State
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

// WebUI configures the read-only browser route map view.
type WebUI struct {
	// Basemaps are the cartographies the reader may switch the map between, in
	// the order they are offered. The first is what a browser that has never
	// chosen one loads. At least one is required, because the map has to paint
	// on something.
	Basemaps []Basemap
}

// Basemap is one cartography the map can load.
type Basemap struct {
	// Name labels the entry in the picker and is the identity a browser
	// remembers its choice by, so it is required and unique across the list.
	Name string

	// StyleURL is the MapLibre style document the operator's browser loads. It
	// is deliberately not a secret: it is served to the page and is visible to
	// anyone who can reach the UI. The default is a keyless provider, so no
	// credential is exposed. A provider that requires an API key would place it
	// in this URL's query and thereby publish it to the browser.
	StyleURL string

	// StyleURLDark is loaded in place of StyleURL when the browser reports a
	// dark system colour scheme. It must share this entry's StyleURL origin. An
	// empty value keeps one style in both schemes.
	StyleURLDark string

	// DarkCartography says this entry's ground is dark whatever the system
	// colour scheme is, which is what satellite imagery is. Anything the page
	// paints over the map reads this rather than the scheme, because a route
	// drawn in the dark-ground ink over light cartography — or the reverse —
	// is the one that cannot be seen.
	//
	// It contradicts StyleURLDark: a provider publishing a dark twin has light
	// cartography to switch away from. Configuring both is refused.
	DarkCartography bool
}

// Surface configures the local map the road surface of a stage is read from.
type Surface struct {
	// Regions are the OpenStreetMap extracts to index, as Geofabrik slugs such
	// as "europe/germany/rheinland-pfalz". They have to cover the ground the
	// operator actually rides: a stage outside every configured region is served
	// without a surface rather than wrongly.
	//
	// An empty list switches surface classification off entirely. Nothing is
	// downloaded, nothing is built, and stages simply carry no surface — which
	// is also what a deployment gets by default, because the right regions are
	// a property of where somebody rides and cannot be guessed.
	Regions []string
	// RebuildInterval is how often the index is rebuilt from freshly published
	// extracts. A rebuild that finds every extract unchanged costs one small
	// request per region and stops there.
	RebuildInterval time.Duration
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
	// StaleAfter bounds how long the trusted source inventory may go without a
	// successful refresh before it is reported as stale and notified. Defaults
	// to 24h.
	StaleAfter time.Duration
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

// Notifications configures supported notification destinations and how much
// routine success reaches them.
type Notifications struct {
	Pushover Pushover
	Success  SuccessNotifications
}

// SuccessNotifications controls routine-success delivery. It never governs a
// failure, a blocked run, or the first success that ends one: those are the
// signals the operator installed notifications for.
type SuccessNotifications struct {
	Policy SuccessPolicy
	// DigestInterval is how much time separates two digests. It is read only by
	// SuccessPolicyDigest.
	DigestInterval time.Duration
}

// SuccessPolicy names what a routine successful run notifies.
type SuccessPolicy string

const (
	// SuccessPolicyEvery pushes one message per successful run.
	SuccessPolicyEvery SuccessPolicy = "every"
	// SuccessPolicyQuiet pushes nothing for a routine success.
	SuccessPolicyQuiet SuccessPolicy = "quiet"
	// SuccessPolicyDigest replaces routine success pushes with one aggregate
	// message per interval.
	SuccessPolicyDigest SuccessPolicy = "digest"
)

// Pushover holds the credentials needed to send a notification, and the origin
// they are sent to. The origin is configurable for the same reason every other
// provider's is: a development or demo environment has to be able to point it at
// an address that goes nowhere, and the alternative is a compiled-in host that
// such an environment would quietly reach with a placeholder token.
type Pushover struct {
	BaseURL          string
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
	Surface       rawSurface       `koanf:"surface"`
	State         rawState         `koanf:"state"`
	Sync          rawSync          `koanf:"sync"`
}

type rawHTTP struct {
	ListenAddress    string `koanf:"listen_address"`
	ReadinessAddress string `koanf:"readiness_address"`
}

type rawWebUI struct {
	Basemaps []rawBasemap `koanf:"basemaps"`
}

type rawBasemap struct {
	Name            string `koanf:"name"`
	StyleURL        string `koanf:"style_url"`
	StyleURLDark    string `koanf:"style_url_dark"`
	DarkCartography bool   `koanf:"dark_cartography"`
}

type rawSurface struct {
	Regions         []string      `koanf:"regions"`
	RebuildInterval time.Duration `koanf:"rebuild_interval"`
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
	StaleAfter            time.Duration `koanf:"stale_after"`
}

type rawNotifications struct {
	Pushover       rawPushover   `koanf:"pushover"`
	SuccessPolicy  string        `koanf:"success_policy"`
	DigestInterval time.Duration `koanf:"digest_interval"`
}

type rawPushover struct {
	BaseURL              string `koanf:"base_url"`
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

	built, buildErr := build(&raw)
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

func build(raw *rawSettings) (*Settings, error) {
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
	if err := validateSurface(raw.Surface); err != nil {
		return nil, err
	}
	if err := validateHTTPSOrigin("notifications.pushover.base_url", raw.Notifications.Pushover.BaseURL); err != nil {
		return nil, err
	}
	if err := validateNotifications(&raw.Notifications); err != nil {
		return nil, err
	}

	basemaps, err := validateBasemaps(raw.WebUI.Basemaps)
	if err != nil {
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
		WebUI: WebUI{
			Basemaps: basemaps,
		},
		Surface: Surface{
			Regions:         trimmedRegions(raw.Surface.Regions),
			RebuildInterval: raw.Surface.RebuildInterval,
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
			StaleAfter:            raw.Sync.StaleAfter,
		},
		Notifications: Notifications{
			Success: SuccessNotifications{
				Policy:         SuccessPolicy(raw.Notifications.SuccessPolicy),
				DigestInterval: raw.Notifications.DigestInterval,
			},
			Pushover: Pushover{
				BaseURL:          raw.Notifications.Pushover.BaseURL,
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

// validateStyleURL accepts an absolute HTTPS style document URL. Unlike the
// service's own endpoints it permits a query string, because providers that
// require an API key carry it there; such a key is published to the browser and
// is the operator's deliberate choice, not a managed secret.
func validateStyleURL(name, value string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute HTTPS URL without credentials or fragment", name)
	}

	return nil
}

// validateBasemaps checks the list the map may be switched between. At least one
// entry is required, because a map with no cartography paints nothing; each is
// named, because the name is both the label the reader picks by and the identity
// a browser remembers its choice by, and a repeated name would make those two
// disagree.
//
// The same-origin rule on a dark twin holds per entry rather than across the
// list, and that is the whole of what this change widened. One entry still
// reaches one third-party origin, and the page still requests only the entry
// currently on screen; what grew is the set of providers the operator has
// offered, which is the point of the setting.
func validateBasemaps(raw []rawBasemap) ([]Basemap, error) {
	if len(raw) == 0 {
		return nil, errors.New("webui.basemaps must contain at least one entry")
	}

	basemaps := make([]Basemap, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for index, entry := range raw {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return nil, fmt.Errorf("webui.basemaps[%d].name is required", index)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("webui.basemaps[%d].name is duplicated", index)
		}
		seen[name] = struct{}{}

		styleURL := strings.TrimSpace(entry.StyleURL)
		if err := validateStyleURL(fmt.Sprintf("webui.basemaps[%d].style_url", index), styleURL); err != nil {
			return nil, err
		}

		styleURLDark := strings.TrimSpace(entry.StyleURLDark)
		if styleURLDark != "" {
			if entry.DarkCartography {
				return nil, fmt.Errorf(
					"webui.basemaps[%d] must not set both style_url_dark and dark_cartography", index)
			}
			darkName := fmt.Sprintf("webui.basemaps[%d].style_url_dark", index)
			if err := validateStyleURL(darkName, styleURLDark); err != nil {
				return nil, err
			}
			if !sameOrigin(styleURLDark, styleURL) {
				return nil, fmt.Errorf("%s must be on the same origin as webui.basemaps[%d].style_url", darkName, index)
			}
		}

		basemaps[index] = Basemap{
			Name:            name,
			StyleURL:        styleURL,
			StyleURLDark:    styleURLDark,
			DarkCartography: entry.DarkCartography,
		}
	}

	return basemaps, nil
}

// sameOrigin reports whether two URLs share a scheme and host. Hosts are
// compared case-insensitively because a host is not case-sensitive, and a
// difference in case would otherwise reject a pair the browser treats as one
// origin.
func sameOrigin(left, right string) bool {
	first, err := url.ParseRequestURI(left)
	if err != nil {
		return false
	}
	second, err := url.ParseRequestURI(right)
	if err != nil {
		return false
	}

	return first.Scheme == second.Scheme && strings.EqualFold(first.Host, second.Host)
}

// regionSlug is one Geofabrik region path, such as
// "europe/germany/rheinland-pfalz".
//
// A slug becomes a path under a fixed download host, so its shape is checked
// here rather than trusted: lowercase segments of letters, digits, and single
// hyphens can introduce neither a host nor a traversal. The index builder
// applies the same rule again before it composes a URL — this one exists so a
// mistyped region is a startup error the operator reads, rather than a download
// that fails a week later.
var regionSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*(?:/[a-z0-9]+(?:-[a-z0-9]+)*)*$`)

// validateSurface accepts a set of regions to index, or no regions at all. An
// empty list is the operator's switch for leaving stages unclassified, so it is
// a valid setting rather than a missing one.
func validateSurface(surface rawSurface) error {
	regions := trimmedRegions(surface.Regions)
	for _, region := range regions {
		if !regionSlug.MatchString(region) {
			return fmt.Errorf(
				"surface.regions entry %q must be a region path such as "+
					"\"europe/germany/rheinland-pfalz\"", region,
			)
		}
	}
	if len(regions) > 0 && surface.RebuildInterval <= 0 {
		return errors.New("surface.rebuild_interval must be positive")
	}

	return nil
}

// trimmedRegions drops blank entries and repeats, keeping the order they were
// written in.
//
// A repeat is dropped rather than rejected because it asks for nothing that is
// not already being done: the second copy of a region downloads the same
// extract and appends the same ways a second time, which cannot change what the
// index answers but does pay for that region twice in build time, memory, and
// size. Silently doing the work once is the useful reading of a typo.
func trimmedRegions(regions []string) []string {
	trimmed := make([]string, 0, len(regions))
	seen := make(map[string]bool, len(regions))
	for _, region := range regions {
		if region = strings.TrimSpace(region); region != "" && !seen[region] {
			seen[region] = true
			trimmed = append(trimmed, region)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}

	return trimmed
}

func validateHTTPSURL(name, value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute HTTPS URL without credentials, query, or fragment", name)
	}

	return nil
}

// validateHTTPSOrigin is validateHTTPSURL plus the absence of a path, which is
// what a setting the client parses as an origin has to be. Rejecting a path here
// rather than letting the client reject it keeps the failure where every other
// configuration failure is: before a listener opens, naming the setting.
func validateHTTPSOrigin(name, value string) error {
	if err := validateHTTPSURL(name, value); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("%s must be an origin, without a path", name)
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

// validateNotifications checks the success policy and the period a digest
// covers. A digest with no positive interval would either never be sent or be
// sent on every run, and neither is what the setting means.
func validateNotifications(notifications *rawNotifications) error {
	switch SuccessPolicy(notifications.SuccessPolicy) {
	case SuccessPolicyEvery, SuccessPolicyQuiet, SuccessPolicyDigest:
	default:
		return errors.New("notifications.success_policy must be every, quiet, or digest")
	}
	// The period is checked whatever the policy reads it. A setting that is only
	// consulted by one policy is still a setting an operator will switch to, and
	// finding out then that it was never valid is the wrong moment.
	if notifications.DigestInterval <= 0 {
		return errors.New("notifications.digest_interval must be positive")
	}
	if notifications.DigestInterval > maxDigestInterval {
		return fmt.Errorf(
			"notifications.digest_interval must not exceed %s, which is as far back as the recorded run history reaches",
			maxDigestInterval,
		)
	}

	return nil
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
	if sync.StaleAfter < time.Second {
		return errors.New("sync.stale_after must be at least 1s")
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
		"sync": map[string]any{
			"empty_source_deletion": string(EmptySourceDeletionDeny),
			"stale_after":           defaultStaleAfter.String(),
		},
		"webui": map[string]any{
			"basemaps": []any{
				map[string]any{
					"name":           defaultBasemapName,
					"style_url":      defaultBasemapStyleURL,
					"style_url_dark": defaultBasemapStyleURLDark,
				},
			},
		},
		"surface": map[string]any{
			"rebuild_interval": defaultRebuildInterval.String(),
		},
		"notifications": map[string]any{
			"success_policy":  string(SuccessPolicyEvery),
			"digest_interval": defaultDigestInterval.String(),
			"pushover": map[string]any{
				"base_url": defaultPushoverURL,
			},
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
