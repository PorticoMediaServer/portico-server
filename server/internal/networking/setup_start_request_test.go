package networking

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSetupStartRequestUsesItsActualPendingRevisionAndProtectedBindings(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	raw := `{"setupRequestId":"d297a1dc-3adc-4b1c-a7f2-fbd4640f69e8","setupRevision":"3","state":"` + secret + `","codeChallenge":"` + secret + `","installationId":"c92fe782-aebc-416a-8734-445ea9a0d6c7"}`
	var input ClaimSetupStartRequest
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatal(err)
	}
	if input.SetupRevision != 3 || input.State != secret || input.InstallationID != "c92fe782-aebc-416a-8734-445ea9a0d6c7" {
		t.Fatal("actual setup bindings were changed")
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", input, input), secret) {
		t.Fatal("protected setup input leaked through formatting")
	}
	for _, bad := range []string{
		strings.Replace(raw, `"setupRevision":"3"`, `"setupRevision":"03"`, 1),
		strings.Replace(raw, `"setupRevision":"3"`, `"setupRevision":3`, 1),
		strings.Replace(raw, `"setupRevision":"3"`, `"setupRevision":"0"`, 1),
		strings.Replace(raw, `"setupRevision":"3"`, `"setupRevision":null`, 1),
		strings.Replace(raw, `"state":"`+secret+`"`, `"state":"`+strings.Repeat("A", 42)+`B"`, 1),
		strings.Replace(raw, `"installationId":"c92fe782-aebc-416a-8734-445ea9a0d6c7"`, `"installationId":null`, 1),
		strings.Replace(raw, `"state":`, `"State":`, 1),
		strings.Replace(raw, `"state":`, `"extra":true,"state":`, 1),
		strings.Replace(raw, `"state":`, `"state":"`+secret+`","state":`, 1),
		strings.Replace(raw, `"setupRequestId":"d297a1dc-3adc-4b1c-a7f2-fbd4640f69e8",`, ``, 1),
	} {
		if err := json.Unmarshal([]byte(bad), &input); err == nil {
			t.Fatal("invalid setup input accepted")
		}
		if input != (ClaimSetupStartRequest{}) {
			t.Fatal("failed decode retained private setup input")
		}
	}
}
