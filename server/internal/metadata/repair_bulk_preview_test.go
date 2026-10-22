package metadata

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "testing"
 "portico.local/server/internal/catalog"
 "portico.local/server/internal/catalogtest"
 "portico.local/server/internal/dbwork"
)

func TestBulkMetadataPreviewUsesActualSnapshotsAndCASDigests(t *testing.T) {
 service,targets,_:=bulkFixture(t)
 ctx:=context.Background()
 request:=catalog.BulkMetadataPreviewRequest{}
 for _,target:=range targets {request.Targets=append(request.Targets,catalog.BulkMetadataTarget{Kind:target.Kind,ContentKind:"movie",ID:target.ID,LibraryID:"movies"})}
 gated,err:=dbwork.BeginSnapshot(ctx,service.db); if err!=nil {t.Fatal(err)}; defer gated.Rollback()
 selection,err:=catalog.New(service.db).BulkMetadataSelectionTx(ctx,gated.Tx(),catalog.Viewer{Profile:"profile",Libraries:[]string{"movies"}},request); if err!=nil {t.Fatal(err)}
 preview,err:=service.BulkPreviewTx(ctx,gated.Tx(),selection,allowAll); if err!=nil {t.Fatal(err)}
 if preview.Total!=3 || len(preview.Targets)!=3 {t.Fatal(preview)}
 for index,target:=range targets {if preview.Targets[index].ID!=target.ID || preview.Targets[index].ExpectedRevision!=target.ExpectedRevision {t.Fatalf("CAS digest/order changed: %+v %+v",preview.Targets[index],target)}}
 year:=preview.Fields["year"]
 if year.State!="mixed" || year.Value!=nil || year.DistinctCount!=3 {t.Fatalf("Mixed fabricated a blank: %+v",year)}
 if _,ok:=preview.Fields["title"]; ok {t.Fatal("non-bulk field offered")}
 foundTitle:=false
 for _,field:=range preview.ExcludedFields {if field.Field=="title" && field.Label=="Title" && !field.Bulk {foundTitle=true}}
 if !foundTitle || len(preview.SingleItemFields)!=len(repairSchema("item","movie")) {t.Fatal("single-item exclusions were guessed or lost",preview.ExcludedFields)}
 if preview.Fields["tags"].Entries[0].Value!="Keep" || preview.Fields["tags"].Entries[0].Count!=1 {t.Fatal(preview.Fields["tags"])}
}
func TestBulkMetadataPreviewQueryReportsMoreThan64DistinctValuesWithoutEnumeratingTargets(t *testing.T) {
 service,db:=editorFixture(t)
 fixture:=catalogtest.New(t,db); ids:=[]string{}
 for index:=0;index<70;index++ {item:=fixture.Movie(fixture.Handle("movies"),fmt.Sprintf("/movies/preview-%d.mkv",index),fmt.Sprintf("Preview %d",index),1900+index); ids=append(ids,item.Public)}
 fixture.Drain()
 raw,_:=json.Marshal(ids)
 selection:=catalog.BulkMetadataSelection{SQL:`SELECT id FROM catalog_entities WHERE public_id IN(SELECT pid_blob(value) FROM json_each(?))`,Args:[]any{string(raw)},Revision:"query-revision"}
 gated,err:=dbwork.BeginSnapshot(context.Background(),db); if err!=nil {t.Fatal(err)}; defer gated.Rollback()
 preview,err:=service.BulkPreviewTx(context.Background(),gated.Tx(),selection,allowAll); if err!=nil {t.Fatal(err)}
 year:=preview.Fields["year"]
 if preview.Total!=70 || len(preview.Targets)!=0 || year.DistinctCount!=70 || len(year.Values)!=64 || !year.Truncated || year.State!="mixed" {t.Fatalf("untruthful bounded aggregate: total=%d targets=%d field=%+v",preview.Total,len(preview.Targets),year)}
}
func TestBulkMetadataPreviewRechecksAuthorityInsideSnapshot(t *testing.T) {
 service,targets,_:=bulkFixture(t)
 gated,err:=dbwork.BeginSnapshot(context.Background(),service.db); if err!=nil {t.Fatal(err)}; defer gated.Rollback()
 raw,_:=json.Marshal([]string{targets[0].ID})
 selection:=catalog.BulkMetadataSelection{SQL:`SELECT id FROM catalog_entities WHERE public_id IN(SELECT pid_blob(value) FROM json_each(?))`,Args:[]any{string(raw)}}
 denied:=errors.New("revoked")
 calls:=0
 _,err=service.BulkPreviewTx(context.Background(),gated.Tx(),selection,func(*sql.Tx)error{calls++;if calls>1 {return denied};return nil})
 if !errors.Is(err,denied) || calls!=2 {t.Fatalf("authority not rechecked: %d %v",calls,err)}
}
