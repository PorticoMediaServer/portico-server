package httpapi

import (
 "encoding/json"
 "testing"
 "portico.local/server/internal/catalog"
 "portico.local/server/internal/metadata"
)

// These exercise New(d)'s real registered contract after root's one-line
// registration and apigen integration. No alternate test-only route is installed.
func TestBulkMetadataPreviewHTTPMatchesOwnerGateAndRealValues(t *testing.T) {
 handler,owner,member,db:=editorHTTPFixtureDB(t)
 a,b:=editorItemPublic(t,db,"Alpha"),editorItemPublic(t,db,"Beta")
 body,_:=json.Marshal(catalog.BulkMetadataPreviewRequest{Targets:[]catalog.BulkMetadataTarget{{Kind:"item",ContentKind:"movie",ID:a,LibraryID:"private-library"},{Kind:"item",ContentKind:"movie",ID:b,LibraryID:"private-library"}}})
 denied:=editorRequest(t,handler,"POST","/v1/metadata/bulk/preview",member,"application/json",body)
 if denied.Code==200 {t.Fatal("member metadata preview was permitted")}
 response:=editorRequest(t,handler,"POST","/v1/metadata/bulk/preview",owner,"application/json",body)
 if response.Code!=200 {t.Fatalf("preview: %d %s",response.Code,response.Body.String())}
 var preview metadata.BulkMetadataPreview
 if err:=json.Unmarshal(response.Body.Bytes(),&preview);err!=nil {t.Fatal(err)}
 if preview.Total!=2 || len(preview.Targets)!=2 || preview.Fields["year"].DistinctCount!=2 || preview.Fields["year"].State!="mixed" {t.Fatal(preview)}
 tl6AssertSpecResponse(t,"POST","/v1/metadata/bulk/preview",response)
}
