package app

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func TestProductContractIsBuildIndependentAndRevalidatedBySemanticETag(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	response, err := client.Get(serverURL + "/api/product-contract")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("product contract status = %d", response.StatusCode)
	}
	cacheControl := response.Header.Get("Cache-Control")
	if cacheControl != "private, no-cache" || !strings.Contains(cacheControl, "no-cache") {
		t.Fatalf("semantic Product Contract must be revalidated, Cache-Control = %q", cacheControl)
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		t.Fatal("semantic Product Contract omitted its content ETag")
	}
	var contract CanonicalProductContract
	if err := json.NewDecoder(response.Body).Decode(&contract); err != nil {
		t.Fatal(err)
	}

	var system struct {
		Compatibility CompatibilityEnvelope `json:"compatibility"`
	}
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/system", nil, &system)
	if status != http.StatusOK {
		t.Fatalf("system status = %d, body: %s", status, body)
	}
	if contract.SemanticIdentity == nil || system.Compatibility.SemanticDocuments["productContract"] != *contract.SemanticIdentity {
		t.Fatalf("System does not advertise the exact Product Contract semantic identity")
	}
	if etag != `"`+contract.SemanticIdentity.Digest+`"` {
		t.Fatalf("Product Contract ETag %q does not match semantic digest %q", etag, contract.SemanticIdentity.Digest)
	}
	revalidation, err := http.NewRequest(http.MethodGet, serverURL+"/api/product-contract", nil)
	if err != nil {
		t.Fatal(err)
	}
	revalidation.Header.Set("If-None-Match", etag)
	revalidated, err := client.Do(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	defer revalidated.Body.Close()
	if revalidated.StatusCode != http.StatusNotModified || revalidated.Header.Get("ETag") != etag {
		t.Fatalf("semantic Product Contract revalidation status=%d ETag=%q", revalidated.StatusCode, revalidated.Header.Get("ETag"))
	}
}
