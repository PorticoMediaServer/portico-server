package playback

import (
	"net"
	"net/http"
	"net/netip"
	"portico.local/server/internal/operations"
	"sort"
	"strings"
)

// NetworkClass is the server's own conclusion about how far this playback has to
// travel. It is never taken from the client: the client only declares a
// transport, and the server decides what that means next to its own locality.
type NetworkClass string

const (
	NetworkLocal    NetworkClass = "local"
	NetworkWiFi     NetworkClass = "wifi"
	NetworkCellular NetworkClass = "cellular"
	NetworkRemote   NetworkClass = "remote"
	NetworkUnknown  NetworkClass = "unknown"
)

// TransportClassHeader is the optional client hint. Absent or unrecognised
// values resolve to unknown, which is a lane of its own and never a failure.
const TransportClassHeader = "X-Portico-Transport-Class"

// TransportClasses is the accepted domain of the header.
var TransportClasses = []string{"wifi", "cellular", "ethernet", "unknown"}

// NormalizeTransportClass folds the header into the accepted domain. "wi-fi"
// and "wired" are accepted spellings because clients on three platforms write
// them differently.
func NormalizeTransportClass(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "wifi", "wi-fi":
		return "wifi"
	case "cellular", "mobile", "wwan":
		return "cellular"
	case "ethernet", "wired":
		return "ethernet"
	}
	return "unknown"
}

// ServerLocality is what the server sees of the client's address.
const (
	LocalityLocal   = "local"
	LocalityRemote  = "remote"
	LocalityUnknown = "unknown"
)

// RequestLocality classifies the peer address. Loopback and private/link-local
// unicast are the LAN; a routable address is remote. A proxied request without a
// usable forwarded address is remote, because a proxy hop is not LAN evidence.
func RequestLocality(remoteAddr string, forwarded string, trusted bool) string {
	candidate := remoteAddr
	if trusted {
		first, _, _ := strings.Cut(forwarded, ",")
		first = strings.TrimSpace(first)
		if first == "" {
			return LocalityRemote
		}
		candidate = first
	}
	if host, _, err := net.SplitHostPort(candidate); err == nil {
		candidate = host
	}
	candidate = strings.Trim(candidate, "[]")
	address, err := netip.ParseAddr(candidate)
	if err != nil {
		return LocalityUnknown
	}
	address = address.Unmap()
	if address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsUnspecified() {
		return LocalityLocal
	}
	return LocalityRemote
}

// ResolveNetworkClass combines server locality with the client's declared
// transport. The LAN wins over an ethernet or unknown declaration; a client that
// says it is on cellular is believed, because only it can know that.
func ResolveNetworkClass(locality, transport string) NetworkClass {
	transport = NormalizeTransportClass(transport)
	if transport == "cellular" {
		return NetworkCellular
	}
	switch locality {
	case LocalityLocal:
		if transport == "wifi" {
			return NetworkWiFi
		}
		return NetworkLocal
	case LocalityRemote:
		return NetworkRemote
	}
	return NetworkUnknown
}

// NetworkClassFromRequest is the whole resolution for one HTTP request.
func NetworkClassFromRequest(r *http.Request, trusted bool) (NetworkClass, string, string) {
	if r == nil {
		return NetworkUnknown, LocalityUnknown, "unknown"
	}
	transport := NormalizeTransportClass(r.Header.Get(TransportClassHeader))
	locality := RequestLocality(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), trusted)
	return ResolveNetworkClass(locality, transport), locality, transport
}

// preferenceLane maps a network class onto the viewer preference lanes, which
// are local, wifi, cellular and unknown. A remote class borrows the unknown lane
// because a viewer has no way to have configured every remote network.
func (c NetworkClass) preferenceLane() string {
	switch c {
	case NetworkLocal:
		return "local"
	case NetworkWiFi:
		return "wifi"
	case NetworkCellular:
		return "cellular"
	}
	return "unknown"
}

// DeliveryClamp records one narrowing the server applied over a viewer's own
// choice. Clamps only ever narrow, and each is reported with the value asked
// for and the value applied so a client can explain itself.
type DeliveryClamp struct {
	Field     string `json:"field"`
	Source    string `json:"source"`
	Requested int64  `json:"requested"`
	Applied   int64  `json:"applied"`
}

// ResolvedDeliveryPolicy is the published decision for one session: what the
// viewer asked for on this network, narrowed by what the owner permits.
type ResolvedDeliveryPolicy struct {
	NetworkClass       NetworkClass    `json:"networkClass"`
	ServerLocality     string          `json:"serverLocality"`
	TransportClass     string          `json:"transportClass"`
	PreferenceLane     string          `json:"preferenceLane"`
	DirectPlay         string          `json:"directPlay"`
	DirectStream       string          `json:"directStream"`
	Transcode          string          `json:"transcode"`
	QualityMode        string          `json:"qualityMode"`
	MaxVideoBitrateBPS int             `json:"maxVideoBitrateBps"`
	MaxAudioBitrateBPS int             `json:"maxAudioBitrateBps"`
	MaxVideoHeight     int             `json:"maxVideoHeight"`
	AllowHDR           bool            `json:"allowHDR"`
	PlanningPolicy     string          `json:"planningPolicy"`
	Clamps             []DeliveryClamp `json:"clamps"`
}

// DeliveryPreferenceReader is the viewer half of the resolution. It is the
// accessor shape operations.PreferenceValues already has, so the caller passes
// the effective values straight through without a conversion step.
type DeliveryPreferenceReader interface {
	Bool(key string) bool
	Int(key string) int
	Text(key string) string
}

// DeliveryModes is the domain of the three delivery preferences.
var DeliveryModes = []string{"allow", "prefer", "never", "require"}

func validDeliveryMode(m string) bool {
	for _, v := range DeliveryModes {
		if v == m {
			return true
		}
	}
	return false
}

func clampInt(field string, requested, ceiling int, clamps *[]DeliveryClamp) int {
	if ceiling <= 0 || requested <= 0 || requested <= ceiling {
		if requested <= 0 {
			return ceiling
		}
		return requested
	}
	*clamps = append(*clamps, DeliveryClamp{Field: field, Source: "server_clamp", Requested: int64(requested), Applied: int64(ceiling)})
	return ceiling
}

// ResolveDeliveryPolicy combines the viewer's lane preferences with the owner's
// clamps. Nil preferences resolve to the registry defaults for that lane, so a
// caller that could not read preferences still gets a coherent policy.
func ResolveDeliveryPolicy(values DeliveryPreferenceReader, class NetworkClass, locality, transport string, cfg DeliveryConfiguration) ResolvedDeliveryPolicy {
	cfg = cfg.Normalized()
	lane := class.preferenceLane()
	out := ResolvedDeliveryPolicy{
		NetworkClass:   class,
		ServerLocality: locality,
		TransportClass: NormalizeTransportClass(transport),
		PreferenceLane: lane,
		DirectPlay:     "prefer",
		DirectStream:   "allow",
		Transcode:      "allow",
		QualityMode:    "automatic",
		AllowHDR:       true,
		PlanningPolicy: cfg.PlanningPolicy,
		Clamps:         []DeliveryClamp{},
	}
	if values != nil {
		if m := values.Text("delivery.directPlay"); validDeliveryMode(m) {
			out.DirectPlay = m
		}
		if m := values.Text("delivery.directStream"); validDeliveryMode(m) {
			out.DirectStream = m
		}
		if m := values.Text("delivery.transcode"); validDeliveryMode(m) {
			out.Transcode = m
		}
		prefix := "quality." + lane + "."
		if m := values.Text(prefix + "mode"); m != "" {
			out.QualityMode = m
		}
		out.MaxVideoBitrateBPS = values.Int(prefix+"maxVideoBitrateMbps") * 1_000_000
		out.MaxAudioBitrateBPS = values.Int(prefix+"maxAudioBitrateKbps") * 1_000
		out.MaxVideoHeight = values.Int(prefix + "maxVideoHeight")
		if height := operations.VideoPresetHeight(values.Int(prefix + "maxVideoBitrateMbps")); height > 0 && (out.MaxVideoHeight == 0 || height < out.MaxVideoHeight) {
			out.MaxVideoHeight = height
		}
		out.AllowHDR = values.Bool(prefix + "allowHDR")
	}
	out.MaxVideoBitrateBPS = clampInt("maxVideoBitrateBps", out.MaxVideoBitrateBPS, cfg.MaxVideoBitrateBPS, &out.Clamps)
	out.MaxAudioBitrateBPS = clampInt("maxAudioBitrateBps", out.MaxAudioBitrateBPS, cfg.MaxAudioBitrateBPS, &out.Clamps)
	out.MaxVideoHeight = clampInt("maxVideoHeight", out.MaxVideoHeight, cfg.MaxVideoHeight, &out.Clamps)
	if out.AllowHDR && !cfg.AllowHDR {
		out.Clamps = append(out.Clamps, DeliveryClamp{Field: "allowHDR", Source: "server_clamp", Requested: 1, Applied: 0})
		out.AllowHDR = false
	}
	if !cfg.RemuxEnabled && out.DirectStream != "never" {
		out.Clamps = append(out.Clamps, DeliveryClamp{Field: "directStream", Source: "owner_remux_disabled"})
		out.DirectStream = "never"
	}
	sort.SliceStable(out.Clamps, func(i, j int) bool { return out.Clamps[i].Field < out.Clamps[j].Field })
	return out
}
