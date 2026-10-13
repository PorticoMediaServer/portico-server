// Package i18n supplies the Hosted-only sign-in page's string catalogue.
package i18n

import (
	_ "embed"
	"encoding/json"
)

//go:embed app-sign-in.en.json
var appSignInJSON string

var AppSignIn = func() map[string]string {
	var out map[string]string
	if e := json.Unmarshal([]byte(appSignInJSON), &out); e != nil {
		panic(e)
	}
	return out
}()

func AppSignInJSON() string { return appSignInJSON }
