package administration

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LiveDefaults are the server-wide live source settings every source inherits
// until it overrides them.
type LiveDefaults struct {
	StreamBufferSeconds int    `json:"streamBufferSeconds"`
	RetryWindowSeconds  int    `json:"retryWindowSeconds"`
	UserAgent           string `json:"userAgent"`
	GuideDays           int    `json:"guideDays"`
	// LogoImport turns on fetching channel logos the source advertises.
	LogoImport bool `json:"logoImport"`
	// DiscoveryEnabled allows the HDHomeRun search to send packets on the LAN.
	DiscoveryEnabled bool `json:"discoveryEnabled"`
}

// DefaultLiveDefaults is the answer before any owner write.
func DefaultLiveDefaults() LiveDefaults {
	return LiveDefaults{StreamBufferSeconds: 8, RetryWindowSeconds: 60, UserAgent: "Portico/1.0", GuideDays: 7, LogoImport: true, DiscoveryEnabled: true}
}

func validateLiveDefaults(v *LiveDefaults) error {
	if !safeText(v.UserAgent, 200) || strings.ContainsAny(v.UserAgent, "\r\n") {
		return invalid("settings.userAgent")
	}
	clampInt(&v.StreamBufferSeconds, 0, 120)
	clampInt(&v.RetryWindowSeconds, 5, 3600)
	clampInt(&v.GuideDays, 1, 14)
	return nil
}

// LiveSourceKinds are the source shapes this page configures. A standalone
// XMLTV source carries guide data only: it publishes programmes for channels
// that come from somewhere else and never offers a stream of its own.
var LiveSourceKinds = []string{"playlist", "xmltv-guide", "hdhomerun"}

// FilterRule is one include/exclude list applied to a source's channels.
type FilterRule struct {
	Mode   string   `json:"mode"`
	Values []string `json:"values"`
}

// SourceFilters narrows what a source contributes. Empty value lists mean the
// filter is not applied at all, whatever the mode says.
type SourceFilters struct {
	Categories FilterRule `json:"categories"`
	Countries  FilterRule `json:"countries"`
	Keywords   FilterRule `json:"keywords"`
}

// LogoImport controls pulling channel logos out of the source.
type LogoImport struct {
	Enabled           bool `json:"enabled"`
	OverwriteExisting bool `json:"overwriteExisting"`
}

// ChannelNumbering renumbers a source's channels. "source" keeps what the
// source published; "sequential" assigns startAt, startAt+step, and so on in
// the source's own order.
type ChannelNumbering struct {
	Mode    string `json:"mode"`
	StartAt int    `json:"startAt"`
	Step    int    `json:"step"`
}

// LiveSourceSettings is one source's configuration document.
type LiveSourceSettings struct {
	Kind string `json:"kind"`
	// Null-able overrides: a nil value inherits the server default.
	StreamBufferSeconds *int             `json:"streamBufferSeconds"`
	RetryWindowSeconds  *int             `json:"retryWindowSeconds"`
	UserAgent           string           `json:"userAgent"`
	Filters             SourceFilters    `json:"filters"`
	Logos               LogoImport       `json:"logoImport"`
	Numbering           ChannelNumbering `json:"channelNumbering"`
	// GuideSourceID names the XMLTV source that supplies this source's guide,
	// which is how a standalone guide is attached to a streaming playlist.
	GuideSourceID string `json:"guideSourceId"`
}

// DefaultLiveSourceSettings is the answer before any owner write.
func DefaultLiveSourceSettings() LiveSourceSettings {
	return LiveSourceSettings{
		Kind:      "playlist",
		Filters:   SourceFilters{FilterRule{"include", []string{}}, FilterRule{"include", []string{}}, FilterRule{"exclude", []string{}}},
		Logos:     LogoImport{Enabled: true},
		Numbering: ChannelNumbering{Mode: "source", StartAt: 100, Step: 1},
	}
}

func normalizeFilter(r *FilterRule) {
	if r.Values == nil {
		r.Values = []string{}
	}
	if r.Mode == "" {
		r.Mode = "include"
	}
	seen, kept := map[string]bool{}, make([]string, 0, len(r.Values))
	for _, v := range r.Values {
		key := strings.ToLower(strings.TrimSpace(v))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, strings.TrimSpace(v))
	}
	sort.Strings(kept)
	r.Values = kept
}

func normalizeLiveSourceSettings(v *LiveSourceSettings) {
	normalizeFilter(&v.Filters.Categories)
	normalizeFilter(&v.Filters.Countries)
	normalizeFilter(&v.Filters.Keywords)
}

func validateFilter(r FilterRule, field string, fields *[]string) {
	if !oneOf(r.Mode, "include", "exclude") || len(r.Values) > 200 {
		*fields = append(*fields, field)
		return
	}
	for _, value := range r.Values {
		if value == "" || !safeText(value, 120) {
			*fields = append(*fields, field)
			return
		}
	}
}

func validateLiveSourceSettings(v *LiveSourceSettings) error {
	fields := []string{}
	if !oneOf(v.Kind, LiveSourceKinds...) {
		fields = append(fields, "settings.kind")
	}
	if !safeText(v.UserAgent, 200) || strings.ContainsAny(v.UserAgent, "\r\n") {
		fields = append(fields, "settings.userAgent")
	}
	validateFilter(v.Filters.Categories, "settings.filters.categories", &fields)
	validateFilter(v.Filters.Countries, "settings.filters.countries", &fields)
	validateFilter(v.Filters.Keywords, "settings.filters.keywords", &fields)
	if !oneOf(v.Numbering.Mode, "source", "sequential") {
		fields = append(fields, "settings.channelNumbering.mode")
	}
	if !safeText(v.GuideSourceID, 128) {
		fields = append(fields, "settings.guideSourceId")
	}
	// A guide-only source has nothing to stream, so stream tuning is meaningless
	// on it and is refused rather than silently kept.
	if v.Kind == "xmltv-guide" && (v.StreamBufferSeconds != nil || v.GuideSourceID != "") {
		fields = append(fields, "settings.kind")
	}
	if v.StreamBufferSeconds != nil && (*v.StreamBufferSeconds < 0 || *v.StreamBufferSeconds > 120) {
		fields = append(fields, "settings.streamBufferSeconds")
	}
	if v.RetryWindowSeconds != nil && (*v.RetryWindowSeconds < 5 || *v.RetryWindowSeconds > 3600) {
		fields = append(fields, "settings.retryWindowSeconds")
	}
	if len(fields) > 0 {
		return invalid(fields...)
	}
	clampInt(&v.Numbering.StartAt, 1, 99999)
	clampInt(&v.Numbering.Step, 1, 1000)
	return nil
}

const liveScope = "live"

func liveSourceScope(id string) string { return scopeFor("live-source", id) }

// LiveDefaultsDocument reads the server-wide live settings.
func (s *Service) LiveDefaultsDocument(ctx context.Context, auth Authorize) (Document[LiveDefaults], error) {
	return loadDocument(ctx, s, auth, liveScope, DefaultLiveDefaults(), nil)
}

// SaveLiveDefaults writes the server-wide live settings.
func (s *Service) SaveLiveDefaults(ctx context.Context, auth Authorize, change Change[LiveDefaults]) (Document[LiveDefaults], error) {
	return saveDocument(ctx, s, auth, liveScope, DefaultLiveDefaults(), change, validateLiveDefaults, nil)
}

// LiveSourceDocument is one source's configuration plus the effective values
// after the server defaults are applied, so a client never recomputes them.
type LiveSourceDocument struct {
	SourceID   string             `json:"sourceId"`
	SourceName string             `json:"sourceName"`
	Revision   int64              `json:"revision"`
	Digest     string             `json:"digest"`
	Settings   LiveSourceSettings `json:"settings"`
	Effective  LiveDefaults       `json:"effective"`
	Kinds      []string           `json:"availableKinds"`
	TunerCount int                `json:"tunerCount"`
}

func liveSourceExists(ctx context.Context, tx *sql.Tx, id string) (string, int, error) {
	var name string
	var tuners int
	// A server with no live channel store installed has no sources at all; that
	// is a missing resource rather than a failure.
	if !tableExists(ctx, tx, "live_sources") {
		return "", 0, ErrNotFound
	}
	err := tx.QueryRowContext(ctx, `SELECT name,tuner_count FROM live_sources WHERE id=?`, id).Scan(&name, &tuners)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return name, tuners, err
}

func (s *Service) liveSourceDocument(ctx context.Context, tx *sql.Tx, id string) (LiveSourceDocument, error) {
	out := LiveSourceDocument{SourceID: id, Kinds: LiveSourceKinds}
	name, tuners, err := liveSourceExists(ctx, tx, id)
	if err != nil {
		return out, err
	}
	out.SourceName, out.TunerCount = name, tuners
	defaults, _, err := readDocument(ctx, tx, liveScope, DefaultLiveDefaults())
	if err != nil {
		return out, err
	}
	settings, revision, err := readDocument(ctx, tx, liveSourceScope(id), DefaultLiveSourceSettings())
	if err != nil {
		return out, err
	}
	normalizeLiveSourceSettings(&settings)
	effective := defaults
	if settings.StreamBufferSeconds != nil {
		effective.StreamBufferSeconds = *settings.StreamBufferSeconds
	}
	if settings.RetryWindowSeconds != nil {
		effective.RetryWindowSeconds = *settings.RetryWindowSeconds
	}
	if settings.UserAgent != "" {
		effective.UserAgent = settings.UserAgent
	}
	effective.LogoImport = defaults.LogoImport && settings.Logos.Enabled
	out.Settings, out.Revision, out.Digest, out.Effective = settings, revision, digestOf(settings), effective
	return out, nil
}

// LiveSourceConfiguration reads one source's configuration page.
func (s *Service) LiveSourceConfiguration(ctx context.Context, auth Authorize, id string) (LiveSourceDocument, error) {
	var out LiveSourceDocument
	err := s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		var err error
		out, err = s.liveSourceDocument(ctx, tx, id)
		return err
	})
	return out, err
}

// SaveLiveSourceConfiguration writes one source's configuration.
func (s *Service) SaveLiveSourceConfiguration(ctx context.Context, auth Authorize, id string, change Change[LiveSourceSettings]) (LiveSourceDocument, error) {
	var out LiveSourceDocument
	if !validOperationID(change.OperationID) {
		return out, ErrInput
	}
	value := change.Settings
	normalizeLiveSourceSettings(&value)
	if err := validateLiveSourceSettings(&value); err != nil {
		return out, err
	}
	scope := liveSourceScope(id)
	digest := digestOf(struct {
		Expected int64              `json:"expected"`
		Settings LiveSourceSettings `json:"settings"`
	}{change.ExpectedRevision, value})
	err := s.transaction(ctx, auth, func(tx *sql.Tx) error {
		stored, replayed, err := receipt[LiveSourceDocument](ctx, tx, scope, change.OperationID, digest)
		if err != nil {
			return err
		}
		if replayed {
			out = stored
			return nil
		}
		if _, _, err = liveSourceExists(ctx, tx, id); err != nil {
			return err
		}
		if value.GuideSourceID != "" && value.GuideSourceID != id {
			if _, _, err = liveSourceExists(ctx, tx, value.GuideSourceID); err != nil {
				return invalid("settings.guideSourceId")
			}
		}
		_, revision, err := readDocument(ctx, tx, scope, DefaultLiveSourceSettings())
		if err != nil {
			return err
		}
		if revision != change.ExpectedRevision {
			return ErrConflict
		}
		at := s.milliseconds()
		if err = writeDocument(ctx, tx, scope, revision+1, value, at); err != nil {
			return err
		}
		if out, err = s.liveSourceDocument(ctx, tx, id); err != nil {
			return err
		}
		return saveReceipt(ctx, tx, scope, change.OperationID, digest, out, at)
	})
	return out, err
}

// ChannelMapEntry is one channel of a source with the owner's overrides applied.
type ChannelMapEntry struct {
	ChannelID string `json:"channelId"`
	Name      string `json:"name"`
	// SourceNumber is what the source published; Number is what the server
	// serves, which is the override when one exists.
	SourceNumber   string `json:"sourceNumber"`
	Number         string `json:"number"`
	Group          string `json:"group"`
	GuideChannelID string `json:"guideChannelId"`
	Hidden         bool   `json:"hidden"`
	Position       int    `json:"position"`
	Overridden     bool   `json:"overridden"`
}

// ChannelMapPage is one page of a source's channel map.
type ChannelMapPage struct {
	SourceID   string            `json:"sourceId"`
	Generation string            `json:"generation"`
	Items      []ChannelMapEntry `json:"items"`
	NextCursor string            `json:"nextCursor"`
}

// channelMapPage reads one page inside a caller's transaction, so a write can
// return the page it just produced rather than reading it again afterwards.
func channelMapPage(ctx context.Context, tx *sql.Tx, id string, after int64, size int) (ChannelMapPage, error) {
	out := ChannelMapPage{SourceID: id, Items: []ChannelMapEntry{}}
	if _, _, err := liveSourceExists(ctx, tx, id); err != nil {
		return out, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT active_generation FROM live_sources WHERE id=?`, id).Scan(&out.Generation); err != nil {
		return out, err
	}
	if out.Generation == "" {
		return out, nil
	}
	settings, _, err := readDocument(ctx, tx, liveSourceScope(id), DefaultLiveSourceSettings())
	if err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT v.channel_id,v.name,v.number,v.group_name,v.position,COALESCE(n.number,''),COALESCE(n.guide_channel_id,''),COALESCE(n.hidden,0) FROM live_channel_versions v LEFT JOIN admin_channel_numbers n ON n.source_id=? AND n.channel_id=v.channel_id WHERE v.generation_id=? AND v.position>? ORDER BY v.position LIMIT ?`, id, out.Generation, after, size+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e ChannelMapEntry
		var override string
		var hidden int
		if err = rows.Scan(&e.ChannelID, &e.Name, &e.SourceNumber, &e.Group, &e.Position, &override, &e.GuideChannelID, &hidden); err != nil {
			return out, err
		}
		e.Hidden = hidden != 0
		e.Number = e.SourceNumber
		if settings.Numbering.Mode == "sequential" {
			e.Number = strconv.Itoa(settings.Numbering.StartAt + e.Position*settings.Numbering.Step)
		}
		if override != "" {
			e.Number, e.Overridden = override, true
		}
		out.Items = append(out.Items, e)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > size {
		out.Items = out.Items[:size]
		out.NextCursor = encodeCursor("channel-map", int64(out.Items[size-1].Position), out.Items[size-1].ChannelID)
	}
	return out, nil
}

// ChannelMap pages the active generation's channels with their overrides.
func (s *Service) ChannelMap(ctx context.Context, auth Authorize, id, token string, limit int) (ChannelMapPage, error) {
	out := ChannelMapPage{SourceID: id, Items: []ChannelMapEntry{}}
	size, err := pageSize(limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor("channel-map", token)
	if err != nil {
		return out, err
	}
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		var err error
		out, err = channelMapPage(ctx, tx, id, c.Order, size)
		return err
	})
	return out, err
}

// ChannelMapOverride is one owner override in a channel map write.
type ChannelMapOverride struct {
	ChannelID      string `json:"channelId"`
	Number         string `json:"number"`
	GuideChannelID string `json:"guideChannelId"`
	Hidden         bool   `json:"hidden"`
}

// ChannelMapChange is the channel map write. An override with an empty number,
// empty guide mapping and hidden false clears the row.
type ChannelMapChange struct {
	ExpectedRevision int64                `json:"expectedRevision"`
	OperationID      string               `json:"operationId"`
	Overrides        []ChannelMapOverride `json:"overrides"`
}

func validChannelNumber(v string) bool {
	if v == "" {
		return true
	}
	if len(v) > 12 {
		return false
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9' || r == '.') {
			return false
		}
	}
	return true
}

// SaveChannelMap applies per-channel renumbering and guide mapping. The source
// configuration revision fences it, so a renumber cannot land on top of a kind
// or numbering-mode change the caller never saw.
func (s *Service) SaveChannelMap(ctx context.Context, auth Authorize, id string, change ChannelMapChange) (ChannelMapPage, error) {
	out := ChannelMapPage{SourceID: id, Items: []ChannelMapEntry{}}
	if !validOperationID(change.OperationID) || len(change.Overrides) > MaxPageSize {
		return out, ErrInput
	}
	numbers := map[string]bool{}
	for _, o := range change.Overrides {
		if o.ChannelID == "" || !safeText(o.ChannelID, 200) || !validChannelNumber(o.Number) || !safeText(o.GuideChannelID, 200) {
			return out, invalid("overrides")
		}
		if o.Number != "" {
			if numbers[o.Number] {
				return out, invalid("overrides.number")
			}
			numbers[o.Number] = true
		}
	}
	scope := liveSourceScope(id)
	digest := digestOf(change)
	err := s.transaction(ctx, auth, func(tx *sql.Tx) error {
		stored, replayed, err := receipt[ChannelMapPage](ctx, tx, "channel-map:"+id, change.OperationID, digest)
		if err != nil {
			return err
		}
		if replayed {
			out = stored
			return nil
		}
		if _, _, err = liveSourceExists(ctx, tx, id); err != nil {
			return err
		}
		_, revision, err := readDocument(ctx, tx, scope, DefaultLiveSourceSettings())
		if err != nil {
			return err
		}
		if revision != change.ExpectedRevision {
			return ErrConflict
		}
		at := s.milliseconds()
		for _, o := range change.Overrides {
			if o.Number == "" && o.GuideChannelID == "" && !o.Hidden {
				if _, err = tx.ExecContext(ctx, `DELETE FROM admin_channel_numbers WHERE source_id=? AND channel_id=?`, id, o.ChannelID); err != nil {
					return err
				}
				continue
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO admin_channel_numbers VALUES(?,?,?,?,?,?) ON CONFLICT(source_id,channel_id) DO UPDATE SET number=excluded.number,guide_channel_id=excluded.guide_channel_id,hidden=excluded.hidden,updated_ms=excluded.updated_ms`, id, o.ChannelID, o.Number, o.GuideChannelID, boolToInt(o.Hidden), at); err != nil {
				return err
			}
		}
		// The overrides are configuration for this source, so they move its
		// revision: a client that renumbered must reload before the next write.
		settings, _, err := readDocument(ctx, tx, scope, DefaultLiveSourceSettings())
		if err != nil {
			return err
		}
		if err = writeDocument(ctx, tx, scope, revision+1, settings, at); err != nil {
			return err
		}
		if out, err = channelMapPage(ctx, tx, id, 0, DefaultPageSize); err != nil {
			return err
		}
		return saveReceipt(ctx, tx, "channel-map:"+id, change.OperationID, digest, out, at)
	})
	return out, err
}

// TunerDevice is one discovered network tuner.
type TunerDevice struct {
	DeviceID        string `json:"deviceId"`
	Model           string `json:"model"`
	FriendlyName    string `json:"friendlyName"`
	Address         string `json:"address"`
	BaseURL         string `json:"baseUrl"`
	LineupURL       string `json:"lineupUrl"`
	TunerCount      int    `json:"tunerCount"`
	FirmwareVersion string `json:"firmwareVersion"`
	// Configured is true when a live source already points at this device.
	Configured bool `json:"configured"`
}

// TunerDiscovery is one discovery sweep's result.
type TunerDiscovery struct {
	Devices  []TunerDevice `json:"devices"`
	Searched string        `json:"searchedFor"`
	Enabled  bool          `json:"discoveryEnabled"`
	// Message explains an empty result rather than leaving a blank list.
	Message string `json:"message,omitempty"`
}

// hdhomerunDiscoveryPort is the UDP port HDHomeRun devices answer on. The
// protocol is a small binary request/response; a device also serves
// http://<address>/discover.json, which is what the result points at.
const hdhomerunDiscoveryPort = 65001

// discoverPacket is the HDHomeRun discovery request: type 0x0002, two TLVs
// (device type = tuner, device id = wildcard) and a CRC.
func discoverPacket() []byte {
	body := []byte{0x01, 0x04, 0x00, 0x00, 0x00, 0x01, 0x02, 0x04, 0xff, 0xff, 0xff, 0xff}
	packet := []byte{0x00, 0x02, byte(len(body) >> 8), byte(len(body))}
	packet = append(packet, body...)
	return append(packet, hdhomerunCRC(packet)...)
}

// hdhomerunCRC is the little-endian CRC-32 the protocol appends to every packet.
func hdhomerunCRC(data []byte) []byte {
	crc := ^uint32(0)
	for _, b := range data {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xedb88320
			} else {
				crc >>= 1
			}
		}
	}
	crc = ^crc
	return []byte{byte(crc), byte(crc >> 8), byte(crc >> 16), byte(crc >> 24)}
}

// DiscoverTuners looks for HDHomeRun devices on the local network. The sweep is
// a bounded UDP broadcast followed by a read of each responder's discover.json;
// it runs only when the owner has left discovery enabled.
func (s *Service) DiscoverTuners(ctx context.Context, auth Authorize, window time.Duration) (TunerDiscovery, error) {
	out := TunerDiscovery{Devices: []TunerDevice{}, Searched: "hdhomerun"}
	if window <= 0 || window > 10*time.Second {
		window = 2 * time.Second
	}
	configured := map[string]bool{}
	err := s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		defaults, _, err := readDocument(ctx, tx, liveScope, DefaultLiveDefaults())
		if err != nil {
			return err
		}
		out.Enabled = defaults.DiscoveryEnabled
		if !tableExists(ctx, tx, "live_sources") {
			return nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT id FROM live_sources`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return err
			}
			configured[id] = true
		}
		return rows.Err()
	})
	if err != nil {
		return out, err
	}
	if !out.Enabled {
		out.Message = "Tuner discovery is switched off in the live source settings."
		return out, nil
	}
	discover := s.Discover
	if discover == nil {
		discover = discoverHDHomeRun
	}
	devices, err := discover(ctx, window)
	if err != nil {
		out.Message = "No tuner answered on this network."
		return out, nil
	}
	for i := range devices {
		devices[i].Configured = configured[devices[i].DeviceID]
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].DeviceID < devices[j].DeviceID })
	out.Devices = devices
	if len(devices) == 0 {
		out.Message = "No tuner answered on this network."
	}
	return out, nil
}

// ssdpSearch is the multicast discovery request for devices that answer SSDP
// rather than the tuner protocol. Both legs are sent from one socket, because a
// device may answer either and an owner should not have to know which.
var ssdpSearch = []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: ssdp:all\r\n\r\n")

// ssdpTuner accepts an SSDP answer only when it identifies itself as a tuner.
// Every other device on the network answers ssdp:all and none of them belong in
// a tuner list.
func ssdpTuner(payload string) bool {
	lower := strings.ToLower(payload)
	if !strings.HasPrefix(lower, "http/1.1 200") {
		return false
	}
	return strings.Contains(lower, "hdhomerun") || strings.Contains(lower, "silicondust")
}

// discoverHDHomeRun broadcasts on every interface that carries a broadcast
// address, which is what makes this work the same on Windows, macOS and Linux
// without a platform-specific socket option. Every responder's own
// discover.json is what the result points at.
func discoverHDHomeRun(ctx context.Context, window time.Duration) ([]TunerDevice, error) {
	connection, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	deadline := time.Now().Add(window)
	if stamp, ok := ctx.Deadline(); ok && stamp.Before(deadline) {
		deadline = stamp
	}
	if err = connection.SetDeadline(deadline); err != nil {
		return nil, err
	}
	packet := discoverPacket()
	for _, target := range broadcastTargets() {
		_, _ = connection.WriteTo(packet, &net.UDPAddr{IP: target, Port: hdhomerunDiscoveryPort})
	}
	_, _ = connection.WriteTo(ssdpSearch, &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900})
	seen, devices := map[string]bool{}, []TunerDevice{}
	buffer := make([]byte, 2048)
	for {
		n, from, err := connection.ReadFrom(buffer)
		if err != nil {
			break
		}
		if n < 8 {
			continue
		}
		payload := string(buffer[:n])
		// A tuner-protocol reply is a binary frame whose first two bytes are the
		// discovery reply type; anything textual is judged as SSDP.
		tuner := buffer[0] == 0x00 && buffer[1] == 0x03
		if !tuner && !ssdpTuner(payload) {
			continue
		}
		host, _, splitErr := net.SplitHostPort(from.String())
		if splitErr != nil || seen[host] {
			continue
		}
		seen[host] = true
		devices = append(devices, TunerDevice{Address: host, BaseURL: "http://" + host, LineupURL: "http://" + host + "/lineup.json", DeviceID: host})
	}
	return devices, nil
}

// broadcastTargets is every interface broadcast address plus the global
// broadcast, so a host with several networks reaches a tuner on any of them.
func broadcastTargets() []net.IP {
	out := []net.IP{net.IPv4bcast}
	interfaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, device := range interfaces {
		if device.Flags&net.FlagUp == 0 || device.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addresses, err := device.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			network, ok := address.(*net.IPNet)
			if !ok || network.IP.To4() == nil {
				continue
			}
			ip := network.IP.To4()
			mask := net.IP(network.Mask).To4()
			if mask == nil {
				continue
			}
			broadcast := net.IPv4(ip[0]|^mask[0], ip[1]|^mask[1], ip[2]|^mask[2], ip[3]|^mask[3])
			if _, ok := netip.AddrFromSlice(broadcast.To4()); ok {
				out = append(out, broadcast)
			}
		}
	}
	return out
}

// LogoImportResult reports a logo import sweep over one source's channels.
type LogoImportResult struct {
	SourceID  string `json:"sourceId"`
	Requested int    `json:"requested"`
	Imported  int    `json:"imported"`
	Skipped   int    `json:"skipped"`
	Message   string `json:"message,omitempty"`
}

// logoCandidates reads the logo URLs a source's current generation advertises.
// The channel version row keeps them in its group/name metadata only when the
// source published them, so an empty result is a real answer.
func (s *Service) logoCandidates(ctx context.Context, tx *sql.Tx, source, generation string) (map[string]string, error) {
	out := map[string]string{}
	if !tableExists(ctx, tx, "live_channel_logos") {
		return out, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT channel_id,url FROM live_channel_logos WHERE generation_id=?`, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var channel, url string
		if err = rows.Scan(&channel, &url); err != nil {
			return nil, err
		}
		out[channel] = url
	}
	return out, rows.Err()
}
