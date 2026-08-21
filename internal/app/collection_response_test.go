package app

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestCollectionResponsesEncodeEmptyItemsAsArrays(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "bounded", value: ListResponse[string]{}},
		{name: "cursor", value: CursorListResponse[string]{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			var document map[string]json.RawMessage
			if err := json.Unmarshal(body, &document); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got := string(document["items"]); got != "[]" {
				t.Fatalf("items = %s, want []", got)
			}
		})
	}
}

func TestCollectionResponsesPreservePopulatedItems(t *testing.T) {
	body, err := json.Marshal(ListResponse[string]{Items: []string{"library"}, Total: 1})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var document struct {
		Items []string `json:"items"`
		Total int      `json:"total"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(document.Items) != 1 || document.Items[0] != "library" || document.Total != 1 {
		t.Fatalf("unexpected populated response: %#v", document)
	}
}

func TestLibrariesEndpointEncodesAnEmptyServerAsAnEmptyCollection(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	if _, err := db.Exec(`DELETE FROM libraries`); err != nil {
		t.Fatalf("clear seeded libraries: %v", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var response ListResponse[Library]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/libraries", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("empty libraries status = %d, body: %s", status, body)
	}
	if response.Items == nil || len(response.Items) != 0 || response.Total != 0 {
		t.Fatalf("empty libraries response = %#v, body: %s", response, body)
	}
	if body == "" {
		t.Fatal("empty libraries response body was empty")
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &document); err != nil {
		t.Fatalf("decode empty libraries response: %v", err)
	}
	if got := string(document["items"]); got != "[]" {
		t.Fatalf("empty libraries items = %s, want []", got)
	}
}
