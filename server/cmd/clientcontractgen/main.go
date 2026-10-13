// clientcontractgen records retained client request protocols that have no
// production handler. These declarations do not enable or advertise server APIs.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"portico.local/apikit"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playbackv1"
)

type Device struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Form       string      `json:"form"`
	NowPlaying *NowPlaying `json:"nowPlaying,omitempty"`
}
type NowPlaying struct {
	ItemID     string `json:"itemId,omitempty"`
	Title      string `json:"title,omitempty"`
	State      string `json:"state,omitempty"`
	PositionMS *int64 `json:"positionMs,omitempty"`
}
type PlayTarget struct {
	ItemID   string               `json:"itemId,omitempty"`
	QueueID  string               `json:"queueId,omitempty"`
	Selector *playbackv1.Selector `json:"selector,omitempty"`
}
type Quality struct {
	Mode string `json:"mode"`
	ID   string `json:"id,omitempty"`
	Step string `json:"step,omitempty"`
}
type Command struct {
	Type       string               `json:"type"`
	Target     *PlayTarget          `json:"target,omitempty"`
	StartFrom  string               `json:"startFrom,omitempty"`
	PositionMS *int64               `json:"positionMs,omitempty"`
	Audio      *string              `json:"audio,omitempty"`
	Subtitles  *string              `json:"subtitles,omitempty"`
	Quality    *Quality             `json:"quality,omitempty"`
	Level      *float64             `json:"level,omitempty"`
	Rate       *float64             `json:"rate,omitempty"`
	Selector   *playbackv1.Selector `json:"selector,omitempty"`
}
type ProfilePolicy struct {
	ServerID         string   `json:"serverId"`
	AllowedLibraries []string `json:"allowedLibraries"`
}
type ProfileEdit struct {
	Name             *string        `json:"name,omitempty"`
	Art              *string        `json:"art,omitempty"`
	PIN              *string        `json:"pin,omitempty"`
	Policy           *ProfilePolicy `json:"policy,omitempty"`
	ExpectedRevision int64          `json:"expectedRevision"`
}
type Deletion struct {
	ID             string           `json:"id,omitempty"`
	ProfileID      string           `json:"profileId"`
	Revision       int64            `json:"revision"`
	Name           string           `json:"name,omitempty"`
	Primary        bool             `json:"primary"`
	Status         string           `json:"status,omitempty"`
	Playlists      *int64           `json:"playlists,omitempty"`
	SavedResources *int64           `json:"savedResources,omitempty"`
	Servers        []DeletionServer `json:"servers,omitempty"`
}
type DeletionServer struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Acknowledged bool   `json:"acknowledged"`
}

func register[Request, Response any](r *apikit.Registry, id, method, path string, status int) {
	limit := int64(0)
	if method != "GET" {
		limit = 65536
	}
	err := apikit.RegisterHTTP[Request, Response](r, apikit.Metadata{ID: id, Method: method, Path: path, Summary: "Retained client request; unavailable in production", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: "default", Cost: apikit.Constant, BodyLimit: limit, Status: status, Errors: []string{"not_found"}})
	if err != nil {
		fail(err)
	}
}
func main() {
	r := apikit.New(func(context.Context, *http.Request, apikit.Metadata) error { return identity.ErrUnauthorized }, nil)
	register[struct{}, struct {
		Items      []Device `json:"items"`
		NextCursor string   `json:"nextCursor,omitempty"`
		HasMore    bool     `json:"hasMore,omitempty"`
	}](r, "get_controllable_devices", "GET", "/v1/me/devices", 200)
	register[Command, struct {
		CommandID string `json:"commandId,omitempty"`
		ID        string `json:"id,omitempty"`
	}](r, "send_device_command", "POST", "/v1/devices/{id}/commands", 202)
	register[struct {
		TargetDeviceID string `json:"targetDeviceId"`
	}, struct {
		TransferID string `json:"transferId"`
	}](r, "offer_session_transfer", "POST", "/v1/playback/sessions/{id}/transfers", 202)
	register[struct{}, struct {
		AccountID   string                   `json:"accountId"`
		MaxProfiles int                      `json:"maxProfiles"`
		Profiles    []identity.DirectProfile `json:"profiles"`
	}](r, "get_account_profiles", "GET", "/v1/account/profiles", 200)
	register[struct {
		Name string `json:"name"`
		Art  string `json:"art"`
	}, struct{}](r, "create_account_profile", "POST", "/v1/account/profiles", 201)
	register[ProfileEdit, struct{}](r, "edit_account_profile", "PATCH", "/v1/account/profiles/{id}", 200)
	register[struct {
		Profiles []struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		} `json:"profiles"`
	}, struct{}](r, "order_account_profiles", "PUT", "/v1/account/profiles/order", 200)
	register[struct{}, struct{}](r, "forget_account_profile_trust", "DELETE", "/v1/account/profiles/{id}/trust", 204)
	register[struct{}, struct {
		Items []ProfilePolicy `json:"items"`
	}](r, "get_account_profile_policies", "GET", "/v1/account/profiles/{id}/policies", 200)
	register[struct{}, Deletion](r, "preview_account_profile_deletion", "GET", "/v1/account/profiles/{id}/deletion-preview", 200)
	register[struct {
		ExpectedRevision     int64 `json:"expectedRevision"`
		DeleteSavedResources bool  `json:"deleteSavedResources"`
	}, Deletion](r, "delete_account_profile", "DELETE", "/v1/account/profiles/{id}", 200)
	register[struct{}, Deletion](r, "get_account_profile_deletion", "GET", "/v1/account/profile-deletions/{id}", 200)
	artifacts, err := apikit.GenerateWith(r.Definitions(), apikit.Options{})
	if err != nil {
		fail(err)
	}
	for name, raw := range map[string][]byte{"runtime.ts": artifacts.Runtime, "index.ts": artifacts.Index, "errors.ts": artifacts.Errors, "limits.ts": artifacts.Limits, "decoders.ts": artifacts.Decoders, "types.ts": artifacts.Typescript, "routes.ts": artifacts.Routes, "schemas.ts": artifacts.Schemas} {
		target := filepath.Join("..", "packages/contracts/src/optional", name)
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			fail(err)
		}
		if err = os.WriteFile(target, raw, 0644); err != nil {
			fail(err)
		}
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
