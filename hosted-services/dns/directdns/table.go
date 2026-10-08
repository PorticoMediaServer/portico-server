package directdns

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Member-only labels are independent of the CT-visible certificate namespace.
func ValidMemberLabel(label string) bool {
	if len(label) != 32 {
		return false
	}
	for _, c := range label {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func CurrentHostname(namespace, zone, label string) string {
	if !ValidMemberLabel(label) {
		return ""
	}
	return "c-" + label + "." + strings.ToLower(namespace) + "." + normalizeZone(zone)
}
func LANHostname(namespace, zone, label string) string {
	return strings.Replace(CurrentHostname(namespace, zone, label), "c-", "l-", 1)
}
func NamespaceFromCurrentHostname(name, zone string) (string, bool) {
	zone, name = normalizeZone(zone), normalizeZone(name)
	label, rest, ok := strings.Cut(name, ".")
	if !ok || len(label) < 2 || (label[:2] != "c-" && label[:2] != "l-") || !ValidMemberLabel(label[2:]) {
		return "", false
	}
	namespace, ok := strings.CutSuffix(rest, "."+zone)
	if !ok || !ValidNamespace(namespace) {
		return "", false
	}
	return namespace, true
}

type Addresses struct {
	IPv4, IPv6       netip.Addr
	Label            string
	LANIPv4, LANIPv6 netip.Addr
	// PreviousLabel still answers until PreviousUntil after a member removal
	// rotated Label (A86), so remaining members keep resolving meanwhile.
	PreviousLabel string    `json:",omitempty"`
	PreviousUntil time.Time `json:",omitzero"`
}

// previousValid reports whether the grace label still answers at now.
func (a Addresses) previousValid(now time.Time) bool {
	return ValidMemberLabel(a.PreviousLabel) && now.Before(a.PreviousUntil)
}

// Feed reads address changes from Hosted. There are only as many readers as DNS nodes, but
// Hosted contact must still be cheap and rare (Justin's rule), so the feed polls on a
// 30–60 s interval with jitter and uses conditional requests: the node's sequence is sent
// as If-None-Match and an unchanged feed answers 304 with no body. Pulling from a
// remembered sequence makes every failure the same case: a restart, a missed change and a
// brand-new node all just resume.
type Feed struct {
	URL       string
	Secret    string
	Interval  time.Duration
	Client    *http.Client
	Table     *Table
	Log       func(string)
	Readiness *Readiness
}

const feedPage = 5000

// feedDefaultInterval is the base poll interval. The actual pause between polls is the base
// plus up to one base of jitter (30–60 s by default), so nodes stay staggered and a newly
// claimed server's address still resolves within about a minute.
const feedDefaultInterval = 30 * time.Second

// FeedETag names a feed position for conditional requests. The ETag of a node at sequence N
// is FeedETag(N); Hosted answers 304 when its head matches the If-None-Match value.
func FeedETag(sequence int64) string {
	return strconv.Quote(strconv.FormatInt(sequence, 10))
}

// parseFeedETag decodes one ETag value (with or without quotes or a weak prefix).
func parseFeedETag(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "W/") {
		value = strings.TrimSpace(value[2:])
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return 0, false
		}
		value = unquoted
	}
	sequence, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || sequence < 0 {
		return 0, false
	}
	return sequence, true
}

// MatchFeedETag reports whether an If-None-Match header value matches the given sequence.
// It accepts a comma-separated list and weak validators; "*" never matches so a generic
// client can never hide feed rows from itself.
func MatchFeedETag(header string, sequence int64) bool {
	if header == "" {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimSpace(candidate) == "*" {
			continue
		}
		if seq, ok := parseFeedETag(candidate); ok && seq == sequence {
			return true
		}
	}
	return false
}

func (f *Feed) Run(ctx context.Context) {
	go func() {
		timer := time.NewTicker(30 * time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if err := f.Table.Compact(); err != nil && f.Log != nil {
					f.Log("DNS table compaction failed")
				}
			}
		}
	}()
	interval := f.Interval
	if interval <= 0 {
		interval = feedDefaultInterval
	}
	failures := 0
	for {
		more, e := f.once(ctx)
		f.Readiness.Record(more, e)
		delay := interval + time.Duration(rand.Int64N(int64(interval)+1))
		switch {
		case e != nil:
			// Hosted being down changes nothing here: the table keeps answering. Back off so a
			// recovering Hosted is not greeted by every node at once.
			failures = min(failures+1, 6)
			delay = time.Duration(1<<failures)*interval + time.Duration(rand.Int64N(int64(interval)))
			if f.Log != nil && failures == 1 {
				f.Log("address feed unavailable: " + e.Error())
			}
		case more:
			failures, delay = 0, 0
		default:
			failures = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (f *Feed) once(ctx context.Context) (bool, error) {
	step, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	sequence := f.Table.Sequence()
	target := f.URL + "?after=" + url.QueryEscape(strconv.FormatInt(sequence, 10)) + "&limit=" + strconv.Itoa(feedPage)
	request, e := http.NewRequestWithContext(step, http.MethodGet, target, nil)
	if e != nil {
		return false, e
	}
	request.Header.Set("Authorization", "Bearer "+f.Secret)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("If-None-Match", FeedETag(sequence))
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, e := client.Do(request)
	if e != nil {
		return false, e
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		// Hosted reports the feed has not moved past our sequence: synced, no page.
		return false, nil
	}
	if response.StatusCode != http.StatusOK {
		return false, errors.New("address feed returned " + strconv.Itoa(response.StatusCode))
	}
	var body struct {
		Items *[]FeedRow `json:"items"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if e = decoder.Decode(&body); e != nil {
		return false, e
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return false, errors.New("invalid trailing address feed data")
	}
	if body.Items == nil || len(*body.Items) > feedPage {
		return false, errors.New("invalid address feed page")
	}
	for _, row := range *body.Items {
		if row.Sequence <= sequence || !ValidNamespace(row.Namespace) {
			return false, errors.New("invalid address feed ordering or namespace")
		}
		sequence = row.Sequence
	}
	if e = f.Table.Apply(*body.Items); e != nil {
		return false, e
	}
	return len(*body.Items) >= feedPage, nil
}
