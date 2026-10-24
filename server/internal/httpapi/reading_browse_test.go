package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"portico.local/server/internal/identity"
)

func TestReadingBrowseHTTPUsesLiveReadyAuthorityForAllThreeSurfaces(t *testing.T) {
	r:=newReadingFixture(t,"book","pdf")
	ctx:=context.Background()
	readingSeriesOwnerField(t,r,r.item,"author","Protected Reading author")
	readingSeriesOwnerField(t,r,r.item,"series","Protected Reading series")
	readingSeriesDrain(t,r)
	h:=New(r.d)
	type target struct {method,path string;body any}
	base:="/v1/libraries/"+r.library
	targets:=[]target{{"GET",base+"/browse-capabilities",nil},{"POST",base+"/browse",map[string]any{"pivot":"books","limit":1}},{"GET",base+"/facets?field=author",nil}}
	check:=func(token string,status int,hidden bool) {
		t.Helper()
		for _,target:=range targets {w:=readingHTTP(t,h,target.method,target.path,token,"",target.body)
			if w.Code!=status {t.Fatalf("%s %s: %d %s",target.method,target.path,w.Code,w.Body.String())}
			if hidden {for _,secret:=range []string{r.item,r.source,"Protected Reading author","Protected Reading series"} {if strings.Contains(w.Body.String(),secret) {t.Fatalf("hidden Reading fact escaped through %s",target.path)}}}
		}
	}
	check(r.f.owner.AccessToken,200,false)
	check("",401,true)
	readingSeriesOwnerField(t,r,r.item,"author","Changed protected author")
	check(r.f.owner.AccessToken,http.StatusPreconditionFailed,true)
	readingSeriesDrain(t,r)
	owner:=readingOwnerLogin(t,r)
	profiles,err:=r.f.id.CreateDirectProfile(ctx,owner.AccountToken,"Browse reader","blue");if err!=nil {t.Fatal(err)}
	var child identity.DirectProfile
	for _,profile:=range profiles.Profiles {if !profile.Primary&&profile.Name=="Browse reader" {child=profile}}
	selected,err:=r.f.id.SelectDirectProfile(ctx,owner.AccountToken,child.ID,identity.DirectSelection{});if err!=nil {t.Fatal(err)}
	check(selected.Session.AccessToken,200,false)
	if _,err=r.f.id.SetProfileRestrictions(ctx,owner.AccountToken,child.ID,identity.ProfileRestrictionEdit{ExpectedRevision:1,AllowUnrated:false});err!=nil {t.Fatal(err)}
	check(selected.Session.AccessToken,200,true)
	if _,err=r.f.id.EditDirectProfile(ctx,owner.AccountToken,child.ID,identity.DirectProfileEdit{ExpectedRevision:child.Revision,Policy:&identity.ProfilePolicy{AllowedLibraries:[]string{}}});err!=nil {t.Fatal(err)}
	check(selected.Session.AccessToken,403,true)
	if err=r.f.id.LogoutToken(ctx,r.f.owner.AccessToken);err!=nil {t.Fatal(err)}
	check(r.f.owner.AccessToken,401,true)
}
