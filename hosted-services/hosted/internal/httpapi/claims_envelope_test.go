package httpapi

import (
 "encoding/json"
 "net/http/httptest"
 "testing"

 "portico.local/hosted/internal/claims"
)

func TestClaimFailureEnvelope(t *testing.T) {
 w:=httptest.NewRecorder();claimFailure(w,claims.ErrInvalid)
 var body struct{Error struct{Code,Message string;Retryable bool}}
 if err:=json.Unmarshal(w.Body.Bytes(),&body);err!=nil {t.Fatal(err)}
 if w.Code!=400 || body.Error.Code!="invalid_request" || body.Error.Message=="" || body.Error.Retryable {t.Fatal(w.Code,w.Body.String())}
}
