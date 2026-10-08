package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/identity"
	"sort"
	"strings"
	"time"
)

const RegistryRevision = "p09.2"

type Field struct {
	ID          string   `json:"id"`
	Group       string   `json:"group"`
	Scope       string   `json:"scope"`
	Overrides   []string `json:"overrideScopes"`
	Type        string   `json:"type"`
	Default     any      `json:"default"`
	Min         *int     `json:"min,omitempty"`
	Max         *int     `json:"max,omitempty"`
	Options     []string `json:"options,omitempty"`
	Save        string   `json:"saveMode"`
	Apply       string   `json:"applicationMode"`
	Consumer    string   `json:"runtimeConsumer"`
	Description string   `json:"description"`
	Permission  string   `json:"permission"`
	Secret      bool     `json:"secret"`
	Retention   string   `json:"retentionClass"`
	Audit       string   `json:"auditClass"`
}

func bounded(v int) *int { return &v }

// Enumerated owner settings publish their accepted values so a client renders a
// real choice list instead of guessing at server policy.
var (
	HardwareBackends      = []string{"auto", "software", "videotoolbox", "vaapi", "qsv", "nvenc", "amf"}
	ToneMappingAlgorithms = []string{"clip", "linear", "gamma", "reinhard", "hable", "mobius"}
	X264Presets           = []string{"ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow"}
	PlanningPolicies      = []string{"maximum_fidelity", "maximum_compatibility", "minimize_server_work"}
	// UpdateChannels are the release channels the updates page reports against.
	UpdateChannels = []string{"stable", "beta"}
	// Administration (workstream G) publishes connectivity and diagnostics
	// policy through this same registry, so the console has one authority for
	// every server setting rather than a second document per feature area.
	RemoteSignInPolicies     = []string{"allow", "owner-only", "off"}
	SecureConnectionPolicies = []string{"preferred", "required"}
	LogLevels                = []string{"error", "warn", "info", "debug"}
	LogCategories            = []string{"server", "playback", "scan", "network", "client"}
)

func oneOf(value string, allowed []string) bool {
	for _, v := range allowed {
		if v == value {
			return true
		}
	}
	return false
}
func Registry() []Field {
	field := func(id, group, kind string, def any, min, max *int, options []string, apply, consumer, description string) Field {
		return Field{ID: id, Group: group, Scope: "server", Overrides: []string{}, Type: kind, Default: def, Min: min, Max: max, Options: options, Save: "explicit-apply", Apply: apply, Consumer: consumer, Description: description, Permission: "owner", Secret: false, Retention: "configuration", Audit: "administrative"}
	}
	return []Field{
		field("name", "identity", "string", "Portico", nil, nil, nil, "hot", "identity.Service.Name", "Name shown when selecting this server."),
		field("transcodingEnabled", "playback", "boolean", true, nil, nil, nil, "next-operation", "playback delivery planning", "Allows new audio/video conversions. Existing playback is not interrupted."),
		field("perAccountCap", "playback", "nullable-integer", nil, bounded(1), bounded(1000000), nil, "next-operation", "playback admission and expired-lease recovery", "Null means unlimited across an account's profiles and devices."),
		field("serverCap", "playback", "nullable-integer", nil, bounded(1), bounded(1000000), nil, "next-operation", "playback admission and expired-lease recovery", "Null means unlimited. Lowering a cap never evicts existing streams."),
		field("hardwareBackend", "transcoding", "enumeration", "auto", nil, nil, HardwareBackends, "next-operation", "delivery planning and conversion launch", "Encoder family used for conversions. Auto picks the first backend that probes successfully and falls back to software."),
		field("hardwareDevice", "transcoding", "string", "", nil, nil, nil, "next-operation", "delivery planning and conversion launch", "Optional device selector for the chosen backend, such as a render node path or adapter index. Empty lets the backend choose."),
		field("hdrToneMapping", "transcoding", "boolean", true, nil, nil, nil, "next-operation", "delivery planning", "Converts HDR sources to SDR when a conversion is required. Costs additional processing per stream."),
		field("hdrToneMappingAlgorithm", "transcoding", "enumeration", "hable", nil, nil, ToneMappingAlgorithms, "next-operation", "delivery planning", "Tone curve used when HDR tone mapping runs."),
		field("x264Preset", "transcoding", "enumeration", "veryfast", nil, nil, X264Presets, "next-operation", "software conversion launch", "Software encoder speed against size. Slower presets need more CPU per stream."),
		field("directStreamRemux", "transcoding", "boolean", true, nil, nil, nil, "next-operation", "delivery planning", "Repackages compatible sources into a supported container without re-encoding video or audio."),
		field("planningPolicy", "transcoding", "enumeration", "maximum_fidelity", nil, nil, PlanningPolicies, "next-operation", "delivery planning", "Chooses how the planner trades picture fidelity, client compatibility and server work."),
		field("throttleBufferSeconds", "transcoding", "integer", 60, bounded(10), bounded(600), nil, "next-operation", "conversion pacing", "How far ahead of the viewer a conversion may run before it pauses."),
		field("playedRetentionSeconds", "transcoding", "integer", 180, bounded(0), bounded(3600), nil, "next-operation", "conversion segment retention", "How long already played segments stay on disk so a small rewind does not restart the conversion."),
		field("temporaryDirectory", "transcoding", "path", "", nil, nil, nil, "restart", "conversion working directory", "Absolute path for conversion working files. Empty uses the server state directory. Readiness is reported by the transcode capacity report."),
		field("maxConcurrentSessions", "transcoding", "integer", 0, bounded(0), bounded(1000), nil, "next-operation", "conversion admission", "Total conversions allowed at once. 0 means unlimited."),
		field("maxHardwareSessions", "transcoding", "integer", 0, bounded(0), bounded(1000), nil, "next-operation", "conversion admission", "Hardware conversions allowed at once. 0 means unlimited."),
		field("maxSoftwareSessions", "transcoding", "integer", 0, bounded(0), bounded(1000), nil, "next-operation", "conversion admission", "Software conversions allowed at once. 0 means unlimited."),
		field("maxBackgroundSessions", "transcoding", "integer", 0, bounded(0), bounded(1000), nil, "next-operation", "background conversion admission", "Background conversions, such as download preparation, allowed at once. 0 means unlimited."),
		field("diagnosticDays", "retention", "integer", 30, bounded(1), bounded(30), nil, "hot", "operations.Prune", "Runtime and client records use separate quotas. Shortening retention prunes on the next maintenance tick."),
		field("notificationDays", "retention", "integer", 180, bounded(1), bounded(180), nil, "hot", "operations.Notify and Prune", "Local profile inbox retention. No Hosted media notifications."),
		field("jobDays", "retention", "integer", 30, bounded(1), bounded(30), nil, "hot", "operations.Prune", "Terminal orchestration history only; domain workers own their artifacts."),
		field("playHistoryDays", "retention", "integer", 0, bounded(0), bounded(36500), nil, "hot", "operations.Prune", "How long the server's play history is kept. 0 keeps every play since the server was set up. Play counts are kept either way."),
		field("home.communityActivityEnabled", "home", "boolean", false, nil, nil, nil, "hot", "catalog home trending row", "Publish a server-wide popular-this-week row built from aggregated play counts across profiles. Off by default."),
		field("updates.releaseChannel", "updates", "enumeration", "stable", nil, nil, UpdateChannels, "hot", "administration updates report", "Which release channel the updates page reports against. Nothing is ever installed automatically."),
		field("updates.feedUrl", "updates", "string", "", nil, nil, nil, "hot", "administration updates report", "Optional release feed this server may read to report whether a newer build exists. Empty means the server only reports the build it is running."),
		field("library.trashRetentionDays", "library", "nullable-integer", 30, bounded(0), bounded(3650), nil, "hot", "administration media deletion", "How long deleted files stay recoverable in the trash by default. 0 removes files immediately; null uses the shipped 30 days. Each library may override this on its own configuration page."),
		field("remoteSignInPolicy", "connectivity", "enumeration", "allow", nil, nil, RemoteSignInPolicies, "next-operation", "sign-in admission", "Who may sign in from outside this network. Owner-only still allows existing remote sessions to finish."),
		field("remoteBitrateLimitKbps", "connectivity", "integer", 0, bounded(0), bounded(200000), nil, "next-operation", "playback delivery planning", "Server-wide ceiling on video bitrate for remote playback, in kbps. 0 means no ceiling."),
		field("secureConnectionsPolicy", "connectivity", "enumeration", "preferred", nil, nil, SecureConnectionPolicies, "hot", "HTTP API admission", "Preferred permits HTTP and HTTPS. Required refuses HTTP API requests on a claimed server."),
		field("lanNetworks", "connectivity", "string-list", []string{}, nil, nil, nil, "hot", "connectivity status and remote classification", "CIDR networks treated as this server's own LAN."),
		field("accessUrls", "connectivity", "string-list", []string{}, nil, nil, nil, "hot", "connectivity status", "HTTPS addresses members' apps may use to reach this server, such as a reverse proxy or VPN name. Published with the server's routes; each app tests them itself."),
		field("trustedProxies", "connectivity", "string-list", []string{}, nil, nil, nil, "hot", "request locality and forwarded client address", "Reverse proxies (addresses or CIDR networks) whose forwarded client address this server believes. Other peers are judged by their own address."),
		field("treatWanAsLan", "connectivity", "boolean", true, nil, nil, nil, "hot", "request locality", "Count requests from this network's own public address as local, for routers that send LAN devices through the public address."),
		field("uploadCapacityKbps", "connectivity", "integer", 0, bounded(0), bounded(10000000), nil, "next-operation", "remote playback admission", "The home connection's upload speed in kbps. New remote streams share about 80% of it and get a lower quality when it is busy. 0 means unknown: no budget."),
		field("pausedSessionTimeoutMinutes", "connectivity", "integer", 0, bounded(0), bounded(1440), nil, "hot", "playback session sweeper", "End video sessions paused for longer than this many minutes, freeing their stream slot. Audio and Live are never ended. 0 means never."),
		field("advertisedInterface", "connectivity", "string", "", nil, nil, nil, "hot", "LAN route publication", "The network interface whose address LAN clients are given. Empty means automatic, which skips container and virtual bridges."),
		field("customCertificatePath", "connectivity", "string", "", nil, nil, nil, "hot", "TLS listener", "Path to a PEM certificate chain for a custom domain. Reloaded when the file changes."),
		field("customCertificateKeyPath", "connectivity", "string", "", nil, nil, nil, "hot", "TLS listener", "Path to the PEM private key for the custom certificate."),
		field("customCertificateDomain", "connectivity", "string", "", nil, nil, nil, "hot", "TLS listener and route publication", "The domain the custom certificate serves. It is published to members as an access URL on the remote access port."),
		field("lanDiscoveryEnabled", "connectivity", "boolean", true, nil, nil, nil, "hot", "LAN discovery advertisement", "Advertise this server on the local network with mDNS/Bonjour. The connectivity status report says whether this host can."),
		field("deviceApprovalRequired", "access", "boolean", false, nil, nil, nil, "next-operation", "playback admission", "Require an administrator to approve a device before it can play anything."),
		field("logLevel", "diagnostics", "enumeration", "info", nil, nil, LogLevels, "hot", "servicelog.Recorder", "How much detail the message log keeps. A debug window raises this temporarily without changing the saved value."),
		field("logRetention", "diagnostics", "log-retention-list", []LogRetention{}, nil, nil, LogCategories, "hot", "servicelog.Recorder prune", "Days of message log history to keep per category. A category with no entry keeps whatever the bounded ring holds."),
	}
	// Viewer preferences are published by their own registry on GET /v1/preferences.
	// Listing them here too would give clients two disagreeing authorities.
}

type Settings struct {
	Name               string `json:"name"`
	TranscodingEnabled bool   `json:"transcodingEnabled"`
	PerAccountCap      *int   `json:"perAccountCap"`
	ServerCap          *int   `json:"serverCap"`
	// Transcoding administration. No domain table mirrors these, so the console
	// document is their single authority and the registry their only contract.
	HardwareBackend         string `json:"hardwareBackend"`
	HardwareDevice          string `json:"hardwareDevice"`
	HDRToneMapping          bool   `json:"hdrToneMapping"`
	HDRToneMappingAlgorithm string `json:"hdrToneMappingAlgorithm"`
	X264Preset              string `json:"x264Preset"`
	DirectStreamRemux       bool   `json:"directStreamRemux"`
	PlanningPolicy          string `json:"planningPolicy"`
	ThrottleBufferSeconds   int    `json:"throttleBufferSeconds"`
	PlayedRetentionSeconds  int    `json:"playedRetentionSeconds"`
	TemporaryDirectory      string `json:"temporaryDirectory"`
	MaxConcurrentSessions   int    `json:"maxConcurrentSessions"`
	MaxHardwareSessions     int    `json:"maxHardwareSessions"`
	MaxSoftwareSessions     int    `json:"maxSoftwareSessions"`
	MaxBackgroundSessions   int    `json:"maxBackgroundSessions"`
	DiagnosticDays          int    `json:"diagnosticDays"`
	NotificationDays        int    `json:"notificationDays"`
	JobDays                 int    `json:"jobDays"`
	// PlayHistoryDays is how long the server's play history is kept; 0, the
	// default, keeps all of it. A document saved before the field existed reads
	// as 0, which is the default.
	PlayHistoryDays int `json:"playHistoryDays"`
	// Aggregated server-wide play counts stay off until an owner opts in.
	CommunityActivityEnabled bool `json:"home.communityActivityEnabled"`
	// Update reporting and the default trash retention are owner administration
	// values; the administration pages read them and never shadow them.
	UpdateChannel string `json:"updates.releaseChannel"`
	UpdateFeedURL string `json:"updates.feedUrl"`
	// Null means the shipped default, so a client that does not send this field
	// never silently switches the trash off.
	TrashRetentionDays *int `json:"library.trashRetentionDays"`
	// Connectivity, access and diagnostics policy (workstream G). The console
	// document is their single authority; the typed /v1/admin/connectivity and
	// /v1/admin/logs endpoints project these same fields.
	RemoteSignInPolicy      string   `json:"remoteSignInPolicy"`
	RemoteBitrateLimitKbps  int      `json:"remoteBitrateLimitKbps"`
	SecureConnectionsPolicy string   `json:"secureConnectionsPolicy"`
	LANNetworks             []string `json:"lanNetworks"`
	AccessURLs              []string `json:"accessUrls"`
	LANDiscoveryEnabled     bool     `json:"lanDiscoveryEnabled"`
	// Network parity with the usual self-hosted servers (25 Sep): proxies that
	// may speak for the client address, the server's own public address counted
	// as the LAN, the owner's upload capacity (the remote stream budget), the
	// paused-session limit, the interface advertised to LAN clients, and an
	// owner-supplied certificate for a custom domain.
	TrustedProxies              []string       `json:"trustedProxies"`
	TreatWANAsLAN               bool           `json:"treatWanAsLan"`
	UploadCapacityKbps          int            `json:"uploadCapacityKbps"`
	PausedSessionTimeoutMinutes int            `json:"pausedSessionTimeoutMinutes"`
	AdvertisedInterface         string         `json:"advertisedInterface"`
	CustomCertificatePath       string         `json:"customCertificatePath"`
	CustomCertificateKeyPath    string         `json:"customCertificateKeyPath"`
	CustomCertificateDomain     string         `json:"customCertificateDomain"`
	DeviceApprovalRequired      bool           `json:"deviceApprovalRequired"`
	LogLevel                    string         `json:"logLevel"`
	LogRetention                []LogRetention `json:"logRetention"`
}

// LogRetention is one message log category's history budget in days.
type LogRetention struct {
	Category string `json:"category"`
	Days     int    `json:"days"`
}

// DefaultSettings is the registry's own answer before any owner write, so a
// fresh database and a document saved before these fields existed agree.
func DefaultSettings() Settings {
	return Settings{Name: "Portico", TranscodingEnabled: true, HardwareBackend: "auto", HDRToneMappingAlgorithm: "hable", X264Preset: "veryfast", DirectStreamRemux: true, PlanningPolicy: "maximum_fidelity", ThrottleBufferSeconds: 60, PlayedRetentionSeconds: 180, DiagnosticDays: 30, NotificationDays: 180, JobDays: 30, UpdateChannel: "stable", TrashRetentionDays: bounded(30),
		RemoteSignInPolicy: "allow", SecureConnectionsPolicy: "preferred", LANNetworks: []string{}, AccessURLs: []string{}, LANDiscoveryEnabled: true, TrustedProxies: []string{}, TreatWANAsLAN: true, LogLevel: "info", LogRetention: []LogRetention{}}
}

type SettingsDocument struct {
	Revision         int64    `json:"revision"`
	Digest           string   `json:"digest"`
	Requested        Settings `json:"requested"`
	Effective        Settings `json:"effective"`
	ActiveRevision   int64    `json:"activeRevision"`
	RestartFields    []string `json:"restartFields"`
	RegistryRevision string   `json:"registryRevision"`
}
type SettingsChange struct {
	ExpectedRevision int64    `json:"expectedRevision"`
	IdempotencyKey   string   `json:"idempotencyKey"`
	Values           Settings `json:"values"`
}

func readSettings(tx *sql.Tx) (SettingsDocument, error) {
	v := DefaultSettings()
	var body string
	var revision int64 = 1
	e := tx.QueryRow(`SELECT revision,body FROM console_documents WHERE scope='server'`).Scan(&revision, &body)
	if e == nil {
		if e = decodeDocument(body, &v); e != nil {
			return SettingsDocument{}, e
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return SettingsDocument{}, e
	}
	if e = tx.QueryRow(`SELECT revision FROM console_settings_revision WHERE singleton=1`).Scan(&revision); e != nil {
		return SettingsDocument{}, e
	}
	// Existing product settings stay single-authority, rather than mirrored values.
	var name string
	e = tx.QueryRow(`SELECT value FROM configuration WHERE key='name'`).Scan(&name)
	if e == nil {
		v.Name = name
	} else if !errors.Is(e, sql.ErrNoRows) {
		return SettingsDocument{}, e
	}
	if e = tx.QueryRow(`SELECT transcoding_enabled,per_account_cap,server_cap FROM playback_owner_policy WHERE singleton=1`).Scan(&v.TranscodingEnabled, &v.PerAccountCap, &v.ServerCap); e != nil {
		return SettingsDocument{}, e
	}
	b, _ := json.Marshal(v)
	return SettingsDocument{revision, Hash(string(b)), v, v, revision, restartFields(v), RegistryRevision}, nil
}

// restartFields names saved values the running process has not adopted. The
// document is applied atomically, so a value is never half-saved; what a client
// needs instead is the registry's applicationMode, which says which fields only
// take effect after a restart. This stays empty rather than guessing at what
// the process adopted at startup, which no table records.
func restartFields(Settings) []string { return []string{} }
func (s *Store) Settings(ctx context.Context, auth Authorize) (out SettingsDocument, e error) {
	e = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error { var err error; out, err = readSettings(tx); return err })
	return
}
func validCap(n *int) bool { return n == nil || *n >= 1 && *n <= 1000000 }

// clamp keeps a numeric transcoding control inside its published bounds rather
// than failing an owner's save: a control that overshoots its range is a client
// rounding problem. Enumerations and paths still reject, because silently
// substituting a backend or a directory would apply a policy nobody chose.
func clamp(v *int, low, high int) {
	if *v < low {
		*v = low
	}
	if *v > high {
		*v = high
	}
}

// validTemporaryDirectory accepts only an absolute, already-clean path without
// control characters. Empty means "use the server state directory".
func validTemporaryDirectory(v string) bool {
	if v == "" {
		return true
	}
	if len(v) > 1024 || !SafeText(v, 1024) || strings.ContainsAny(v, "\n\t") {
		return false
	}
	return filepath.IsAbs(v) && filepath.Clean(v) == v
}
func validateSettings(v *Settings) error {
	fields := []string{}
	if !SafeText(v.Name, 100) || strings.ContainsAny(v.Name, "\n\t") {
		fields = append(fields, "values.name")
	}
	if !validCap(v.PerAccountCap) {
		fields = append(fields, "values.perAccountCap")
	}
	if !validCap(v.ServerCap) {
		fields = append(fields, "values.serverCap")
	}
	if v.DiagnosticDays < 1 || v.DiagnosticDays > 30 {
		fields = append(fields, "values.diagnosticDays")
	}
	if v.NotificationDays < 1 || v.NotificationDays > 180 {
		fields = append(fields, "values.notificationDays")
	}
	if v.JobDays < 1 || v.JobDays > 30 {
		fields = append(fields, "values.jobDays")
	}
	if v.PlayHistoryDays < 0 || v.PlayHistoryDays > 36500 {
		fields = append(fields, "values.playHistoryDays")
	}
	if !oneOf(v.HardwareBackend, HardwareBackends) {
		fields = append(fields, "values.hardwareBackend")
	}
	if v.HardwareDevice != "" && (len(v.HardwareDevice) > 200 || !SafeText(v.HardwareDevice, 200) || strings.ContainsAny(v.HardwareDevice, "\n\t")) {
		fields = append(fields, "values.hardwareDevice")
	}
	if !oneOf(v.HDRToneMappingAlgorithm, ToneMappingAlgorithms) {
		fields = append(fields, "values.hdrToneMappingAlgorithm")
	}
	if !oneOf(v.X264Preset, X264Presets) {
		fields = append(fields, "values.x264Preset")
	}
	if !oneOf(v.PlanningPolicy, PlanningPolicies) {
		fields = append(fields, "values.planningPolicy")
	}
	if !validTemporaryDirectory(v.TemporaryDirectory) {
		fields = append(fields, "values.temporaryDirectory")
	}
	// An omitted channel is the default channel rather than a refusal: a client
	// that does not know this field yet still saves the rest of the document.
	if v.UpdateChannel == "" {
		v.UpdateChannel = "stable"
	}
	if !oneOf(v.UpdateChannel, UpdateChannels) {
		fields = append(fields, "values.updates.releaseChannel")
	}
	// A feed is read by this server, so only an absolute http(s) URL is accepted
	// and nothing else is dereferenced.
	if v.UpdateFeedURL != "" && (len(v.UpdateFeedURL) > 2048 || !SafeText(v.UpdateFeedURL, 2048) || strings.ContainsAny(v.UpdateFeedURL, " \n\t") || !strings.HasPrefix(v.UpdateFeedURL, "https://") && !strings.HasPrefix(v.UpdateFeedURL, "http://")) {
		fields = append(fields, "values.updates.feedUrl")
	}
	if v.TrashRetentionDays != nil && (*v.TrashRetentionDays < 0 || *v.TrashRetentionDays > 3650) {
		fields = append(fields, "values.library.trashRetentionDays")
	}
	fields = append(fields, validateAdministration(v)...)
	if len(fields) > 0 {
		return &ValidationError{fields}
	}
	clamp(&v.ThrottleBufferSeconds, 10, 600)
	clamp(&v.RemoteBitrateLimitKbps, 0, 200000)
	clamp(&v.PlayedRetentionSeconds, 0, 3600)
	for _, target := range []*int{&v.MaxConcurrentSessions, &v.MaxHardwareSessions, &v.MaxSoftwareSessions, &v.MaxBackgroundSessions} {
		clamp(target, 0, 1000)
	}
	return nil
}
func (s *Store) ApplySettings(ctx context.Context, p identity.Principal, auth Authorize, c SettingsChange) (out SettingsDocument, e error) {
	if e = validateSettings(&c.Values); e != nil {
		return out, e
	}
	e = s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		scope := "settings:" + AccountKey(p)
		raw, digest, err := Receipt(tx, scope, c.IdempotencyKey, c, s.now())
		if err != nil {
			return err
		}
		if raw != "" {
			return decodeDocument(raw, &out)
		}
		current, err := readSettings(tx)
		if err != nil {
			return err
		}
		if current.Revision != c.ExpectedRevision {
			return &ConflictError{current.Revision}
		}
		revision := current.Revision + 1
		b, _ := json.Marshal(c.Values)
		if _, err = tx.Exec(`INSERT INTO console_documents VALUES('server',?,?,?) ON CONFLICT(scope) DO UPDATE SET revision=excluded.revision,body=excluded.body,updated_ms=excluded.updated_ms`, revision, string(b), s.now()); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO configuration VALUES('name',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, c.Values.Name); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE playback_owner_policy SET revision=revision+1,transcoding_enabled=?,per_account_cap=?,server_cap=? WHERE singleton=1`, c.Values.TranscodingEnabled, c.Values.PerAccountCap, c.Values.ServerCap); err != nil {
			return err
		}
		// Domain triggers fence legacy writes. This entire authorized transaction is
		// one externally visible console revision, regardless of account-cap count.
		if _, err = tx.Exec(`UPDATE console_settings_revision SET revision=? WHERE singleton=1`, revision); err != nil {
			return err
		}
		if err = Audit(tx, s.now(), AccountKey(p), "settings.apply", "server", revision); err != nil {
			return err
		}
		out, err = readSettings(tx)
		if err != nil {
			return err
		}
		return SaveReceipt(tx, scope, c.IdempotencyKey, digest, out, s.now())
	})
	return
}

type PreferencePatch map[string]any
type PreferenceChange struct {
	ExpectedRevision int64           `json:"expectedRevision"`
	IdempotencyKey   string          `json:"idempotencyKey"`
	Scope            string          `json:"scope"`
	DeviceClass      string          `json:"deviceClass"`
	Values           PreferencePatch `json:"values"`
}
type PreferenceDocument struct {
	Scope          string           `json:"scope"`
	Revision       int64            `json:"revision"`
	Values         PreferenceValues `json:"values"`
	Digest         string           `json:"digest"`
	ActiveRevision int64            `json:"activeRevision"`
}
type PreferenceRegistryDocument struct {
	Revision string            `json:"revision"`
	Fields   []PreferenceField `json:"fields"`
}
type PreferenceSnapshot struct {
	AudioEffects     AudioEffectPreferences     `json:"audioEffects"`
	DeviceClass      string                     `json:"deviceClass"`
	Registry         PreferenceRegistryDocument `json:"registry"`
	Documents        []PreferenceDocument       `json:"documents"`
	Effective        PreferenceValues           `json:"effective"`
	EffectiveSource  map[string]string          `json:"effectiveSource"`
	ClampedFields    []string                   `json:"clampedFields"`
	RegistryRevision string                     `json:"registryRevision"`
}

func readPreferences(tx *sql.Tx, v identity.Viewer, device string, clamped []string) (PreferenceSnapshot, error) {
	out := PreferenceSnapshot{
		DeviceClass:      device,
		Registry:         PreferenceRegistryDocument{Revision: PreferenceRegistryRevision, Fields: PreferenceRegistry()},
		Documents:        []PreferenceDocument{},
		ClampedFields:    []string{},
		RegistryRevision: PreferenceRegistryRevision,
	}
	if !validDeviceClass(device) {
		return out, invalidFields("deviceClass")
	}
	for _, scope := range PreferenceScopes {
		key, e := preferenceScopeKey(v, scope, device)
		if e != nil {
			return out, e
		}
		d := PreferenceDocument{Scope: scope, Revision: 1, Values: PreferenceValues{}}
		var raw string
		e = tx.QueryRow(`SELECT revision,body FROM console_documents WHERE scope=?`, key).Scan(&d.Revision, &raw)
		if e == nil {
			if d.Values, e = decodePreferenceDocument(raw, scope); e != nil {
				return out, e
			}
		} else if !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
		b, _ := json.Marshal(d.Values)
		d.Digest = Hash(string(b))
		d.ActiveRevision = d.Revision
		out.Documents = append(out.Documents, d)
	}
	effective, source, policy, e := EffectivePreferences(tx, v, device)
	if e != nil {
		return out, e
	}
	out.Effective, out.EffectiveSource = effective, source
	out.AudioEffects = effective.AudioEffects()
	seen := map[string]bool{}
	for _, key := range append(append([]string{}, clamped...), policy...) {
		if !seen[key] {
			seen[key] = true
			out.ClampedFields = append(out.ClampedFields, key)
		}
	}
	sort.Strings(out.ClampedFields)
	return out, nil
}
func (s *Store) Preferences(ctx context.Context, p identity.Principal, device string, auth Authorize) (out PreferenceSnapshot, e error) {
	e = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		var err error
		out, err = readPreferences(tx, p.Viewer, device, nil)
		return err
	})
	return
}

// validatePreferencePatch resolves a submitted patch against the registry. An
// unknown key, a key outside the addressed scope, or a value the registry cannot
// accept names that key in the rejection; only numeric ranges clamp.
func validatePreferencePatch(scope string, patch PreferencePatch) (PreferenceValues, []string, []string, error) {
	values, cleared, clamped := PreferenceValues{}, []string{}, []string{}
	if patch == nil {
		return nil, nil, nil, invalidFields("values")
	}
	keys := make([]string, 0, len(patch))
	for key := range patch {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		f, ok := PreferenceFieldFor(key)
		if !ok || !f.allows(scope) {
			return nil, nil, nil, invalidFields("values." + key)
		}
		if patch[key] == nil {
			cleared = append(cleared, key)
			continue
		}
		canonical, wasClamped, e := canonicalPreference(f, patch[key])
		if e != nil {
			return nil, nil, nil, invalidFields("values." + key)
		}
		values[key] = canonical
		if wasClamped {
			clamped = append(clamped, key)
		}
	}
	return values, cleared, clamped, nil
}

// ApplyPreferences merges the submitted keys into one scope document. A null
// value removes that scope's override, so the next lower scope or the registry
// default becomes effective again.
func (s *Store) ApplyPreferences(ctx context.Context, p identity.Principal, auth Authorize, c PreferenceChange) (out PreferenceSnapshot, e error) {
	if !validDeviceClass(c.DeviceClass) {
		return out, invalidFields("deviceClass")
	}
	key, e := preferenceScopeKey(p.Viewer, c.Scope, c.DeviceClass)
	if e != nil {
		return out, invalidFields("scope")
	}
	patch, cleared, clamped, e := validatePreferencePatch(c.Scope, c.Values)
	if e != nil {
		return out, e
	}
	e = s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		raw, digest, err := Receipt(tx, key, c.IdempotencyKey, c, s.now())
		if err != nil {
			return err
		}
		if raw != "" {
			return decodeDocument(raw, &out)
		}
		var revision int64 = 1
		var body string
		err = tx.QueryRow(`SELECT revision,body FROM console_documents WHERE scope=?`, key).Scan(&revision, &body)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if revision != c.ExpectedRevision {
			return &ConflictError{revision}
		}
		merged, err := decodePreferenceDocument(body, c.Scope)
		if err != nil {
			return err
		}
		for field, value := range patch {
			merged[field] = value
		}
		for _, field := range cleared {
			delete(merged, field)
		}
		b, _ := json.Marshal(merged)
		if _, err = tx.Exec(`INSERT INTO console_documents VALUES(?,?,?,?) ON CONFLICT(scope) DO UPDATE SET revision=excluded.revision,body=excluded.body,updated_ms=excluded.updated_ms`, key, revision+1, string(b), s.now()); err != nil {
			return err
		}
		out, err = readPreferences(tx, p.Viewer, c.DeviceClass, clamped)
		if err != nil {
			return err
		}
		return SaveReceipt(tx, key, c.IdempotencyKey, digest, out, s.now())
	})
	return
}
func days(n int) int64 { return int64(time.Duration(n) * 24 * time.Hour / time.Millisecond) }
