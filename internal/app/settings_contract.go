package app

// This file contains the settings contract plumbing. Mutation receipts are
// kept in the server's private settings namespace. They contain
// only a digest, resulting public revision, and a bounded safe response
// snapshot; a request body (and, especially, a provider secret) is never
// persisted in a receipt.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const settingsMutationReceiptPrefix = "__portico_settings_receipt_"

type settingsMutationReceipt struct {
	Fingerprint string          `json:"fingerprint"`
	Revision    string          `json:"revision"`
	CreatedAt   string          `json:"createdAt"`
	ExpiresAt   string          `json:"expiresAt"`
	Response    json.RawMessage `json:"response,omitempty"`
}

func settingsMutationFingerprint(expectedRevision string, groups map[string]json.RawMessage) (string, error) {
	intent := struct {
		ExpectedRevision string                     `json:"expectedRevision"`
		Groups           map[string]json.RawMessage `json:"groups"`
	}{ExpectedRevision: strings.TrimSpace(expectedRevision), Groups: groups}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func settingsMutationReceiptKey(user User, idempotencyKey string) string {
	// The key is intentionally a digest rather than user input.  This bounds
	// the private-key namespace and keeps arbitrary client strings out of DB
	// diagnostics and backup listings.
	scope := strings.TrimSpace(user.ID) + "\x00" + strings.TrimSpace(idempotencyKey)
	sum := sha256.Sum256([]byte(scope))
	return settingsMutationReceiptPrefix + hex.EncodeToString(sum[:])
}

func validateSettingsIdempotencyKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", nil
	}
	if len(key) < 16 || len(key) > 200 {
		return "", errors.New("idempotencyKey must be between 16 and 200 characters")
	}
	for _, r := range key {
		if r < 0x21 || r > 0x7e {
			return "", errors.New("idempotencyKey must contain printable ASCII characters only")
		}
	}
	return key, nil
}

func loadSettingsMutationReceipt(tx *sql.Tx, key string) (settingsMutationReceipt, bool, error) {
	var raw string
	err := tx.QueryRow(`SELECT value_json FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return settingsMutationReceipt{}, false, nil
	}
	if err != nil {
		return settingsMutationReceipt{}, false, err
	}
	var receipt settingsMutationReceipt
	if err := json.Unmarshal([]byte(raw), &receipt); err != nil || receipt.Fingerprint == "" || receipt.Revision == "" {
		return settingsMutationReceipt{}, false, fmt.Errorf("settings idempotency receipt is malformed")
	}
	if receipt.ExpiresAt != "" {
		if expiresAt, parseErr := time.Parse(time.RFC3339Nano, receipt.ExpiresAt); parseErr == nil && !time.Now().UTC().Before(expiresAt) {
			if _, deleteErr := tx.Exec(`DELETE FROM settings WHERE key = ?`, key); deleteErr != nil {
				return settingsMutationReceipt{}, false, deleteErr
			}
			return settingsMutationReceipt{}, false, nil
		}
	}
	return receipt, true, nil
}

func saveSettingsMutationReceipt(tx *sql.Tx, key string, receipt settingsMutationReceipt) error {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, key, string(encoded), receipt.CreatedAt)
	return err
}

func updateSettingsMutationReceiptResponse(tx *sql.Tx, key string, response SettingsDocument) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if len(encoded) > 1_000_000 {
		return errors.New("settings mutation response exceeds receipt limit")
	}
	var raw string
	if err := tx.QueryRow(`SELECT value_json FROM settings WHERE key = ?`, key).Scan(&raw); err != nil {
		return err
	}
	var receipt settingsMutationReceipt
	if err := json.Unmarshal([]byte(raw), &receipt); err != nil {
		return err
	}
	receipt.Response = append(json.RawMessage(nil), encoded...)
	updated, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE settings SET value_json = ? WHERE key = ?`, string(updated), key)
	return err
}

// SettingsRegistryResponse is the machine-readable projection of the
// server-owned field registry. Clients discover scope, runtime ownership,
// validation and application semantics here rather than guessing from labels.
type SettingsRegistryResponse struct {
	Kind             string                  `json:"kind"`
	ContractID       string                  `json:"contractId"`
	ContractRevision string                  `json:"contractRevision"`
	Scope            string                  `json:"scope"`
	Revision         int                     `json:"revision"`
	Groups           []SettingsRegistryGroup `json:"groups"`
	GeneratedAt      string                  `json:"generatedAt"`
}

type SettingsRegistryGroup struct {
	ID                      string                  `json:"id"`
	OwnerScope              string                  `json:"ownerScope"`
	PermittedOverrideScopes []string                `json:"permittedOverrideScopes"`
	RuntimeConsumer         string                  `json:"runtimeConsumer"`
	OperationalHealthSource string                  `json:"operationalHealthSource"`
	Revision                int                     `json:"revision"`
	Fields                  []SettingsRegistryField `json:"fields"`
}

type SettingsRegistryField struct {
	ID                      string   `json:"id"`
	Type                    string   `json:"type"`
	Validation              string   `json:"validation"`
	OutcomeTest             string   `json:"outcomeTest"`
	CanonicalDefault        any      `json:"canonicalDefault,omitempty"`
	DefaultAuthority        string   `json:"defaultAuthority"`
	Dependencies            []string `json:"dependencies"`
	SaveMode                string   `json:"saveMode"`
	ApplicationMode         string   `json:"applicationMode"`
	Permission              string   `json:"permission"`
	Capability              string   `json:"capability"`
	RuntimeConsumer         string   `json:"runtimeConsumer"`
	OperationalHealthSource string   `json:"operationalHealthSource"`
	OperationalStatus       string   `json:"operationalStatus"`
	Secret                  bool     `json:"secret"`
	SecretClassification    string   `json:"secretClassification"`
	RedactionBehavior       string   `json:"redactionBehavior"`
	AuditClass              string   `json:"auditClass"`
	RetentionClass          string   `json:"retentionClass"`
	ContractRevision        string   `json:"contractRevision"`
}

func settingsRegistryResponse() SettingsRegistryResponse {
	groups := make([]SettingsRegistryGroup, 0, len(canonicalSettingRegistry))
	for id, definition := range canonicalSettingRegistry {
		fields := make([]SettingsRegistryField, 0, len(definition.Fields))
		for field, metadata := range definition.Fields {
			projected := SettingsRegistryField{
				ID: id + "." + field, Type: string(metadata.Type), Validation: metadata.Validation,
				OutcomeTest: metadata.OutcomeTest,
				CanonicalDefault: func() any {
					if metadata.Secret {
						return nil
					}
					return metadata.Default
				}(), DefaultAuthority: "server-product",
				Dependencies: append([]string(nil), metadata.Dependencies...), SaveMode: metadata.SaveMode,
				ApplicationMode: metadata.ApplicationMode, Permission: metadata.Permission, Capability: metadata.Capability,
				RuntimeConsumer: metadata.RuntimeConsumer, OperationalHealthSource: settingGroupHealthSource(id), OperationalStatus: metadata.OperationalStatus,
				Secret: metadata.Secret, SecretClassification: func() string {
					if metadata.Secret {
						return "write-only"
					}
					return "ordinary"
				}(), RedactionBehavior: func() string {
					if metadata.Secret {
						return "never-return"
					}
					return "none"
				}(), AuditClass: metadata.AuditClass, RetentionClass: metadata.RetentionClass, ContractRevision: metadata.ContractRevision,
			}
			fields = append(fields, projected)
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].ID < fields[j].ID })
		groups = append(groups, SettingsRegistryGroup{ID: id, OwnerScope: definition.Scope, PermittedOverrideScopes: []string{}, RuntimeConsumer: definition.RuntimeConsumer, OperationalHealthSource: settingGroupHealthSource(id), Revision: 1, Fields: fields})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return SettingsRegistryResponse{Kind: "settings-registry", ContractID: "PC-SETTINGS-OPERATIONS", ContractRevision: "settings-operations.v1", Scope: "server", Revision: 1, Groups: groups, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

func (s *Server) handleSettingsRegistry(w http.ResponseWriter, r *http.Request, user User) {
	if !canViewServerSettings(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view the settings registry.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	w.Header().Set("Cache-Control", "private, no-cache")
	writeJSON(w, http.StatusOK, settingsRegistryResponse())
}
