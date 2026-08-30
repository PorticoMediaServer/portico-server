package app

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"reflect"
	"strings"
	"testing"
)

func TestProductContractBuildIdentityCannotBeReusedAcrossDeployments(t *testing.T) {
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
	if cacheControl != "private, no-store" || !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("build-specific product contract must not be reusable, Cache-Control = %q", cacheControl)
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
	if !reflect.DeepEqual(contract.Compatibility.Build, system.Compatibility.Build) {
		t.Fatalf("fresh contract build %#v does not match System build %#v", contract.Compatibility.Build, system.Compatibility.Build)
	}
}
