package metadata

import (
 "context"
 "database/sql"
 "database/sql/driver"
 "encoding/json"
 "sort"
 "strings"

 "portico.local/server/internal/catalog"
 "portico.local/server/internal/compactcatalog"
 "portico.local/server/internal/thirdparty/sqlite"
)

type BulkPreviewValue struct { Value string `json:"value"`; Count int64 `json:"count"` }
type BulkPreviewField struct {
 State string `json:"state"`
 DistinctCount int64 `json:"distinctCount"`
 Value *string `json:"value,omitempty"`
 Values []BulkPreviewValue `json:"values"`
 Entries []BulkPreviewValue `json:"entries"`
 Locked int64 `json:"locked"`
 Truncated bool `json:"truncated"`
}
type BulkPreviewTarget struct {
 Kind string `json:"kind"`
 ID string `json:"id"`
 LibraryID string `json:"libraryId"`
 ExpectedRevision string `json:"expectedRevision"`
}
type BulkMetadataPreview struct {
 ServerID string `json:"serverId"`
 ViewerFence string `json:"viewerFence"`
 ContentKind string `json:"contentKind"`
 Kind string `json:"kind"`
 Total int64 `json:"total"`
 SelectionRevision string `json:"selectionRevision,omitempty"`
 Schema []RepairFieldSpec `json:"schema"`
 SingleItemFields []RepairFieldSpec `json:"singleItemFields"`
 ExcludedFields []RepairFieldSpec `json:"excludedFields"`
 Fields map[string]BulkPreviewField `json:"fields"`
 Targets []BulkPreviewTarget `json:"targets"`
}
const bulkPreviewValues = 64

type bulkPreviewVector struct {
 Backbone map[string]string `json:"backbone"`
 Details map[string]string `json:"details"`
 Side map[string]string `json:"side"`
 Locks map[string]int `json:"locks"`
 GenreLocked int `json:"genreLocked"`
 Genres []string `json:"genres"`
}
func bulkPreviewGenres(values []string) []string {
 unique:=map[string]bool{}; out:=[]string{}
 for _,value:=range values {if !unique[value] {unique[value]=true; out=append(out,value)}}
 sort.Strings(out); return out
}
func bulkPreviewValue(kind,itemKind,field,raw string)(string,error) {
 var vector bulkPreviewVector
 if err:=json.Unmarshal([]byte(raw),&vector); err!=nil {return "",err}
 if field=="genres" {return encodeList(bulkPreviewGenres(vector.Genres)),nil}
 value:=repairFieldValue(kind,itemKind,field,vector.Backbone,vector.Details,vector.Side,sql.NullInt64{})
 for _,spec:=range repairSchema(kind,itemKind) {if spec.Field==field && spec.Type=="list" {return encodeList(decodeList(value)),nil}}
 return value,nil
}
func init() {
 // SQL's exact DISTINCT uses the same private registry/value/list semantics as
 // the editor. This read-only scalar creates no storage, schema or grant.
 sqlite.MustRegisterDeterministicScalarFunction("portico_bulk_metadata_value",4,func(_ *sqlite.FunctionContext,args []driver.Value)(driver.Value,error){
  values:=make([]string,4)
  for index,arg:=range args {value,ok:=arg.(string); if !ok {return nil,ErrRepairInput}; values[index]=value}
  return bulkPreviewValue(values[0],values[1],values[2],values[3])
 })
}

type previewAccumulator struct { first string; hasFirst bool; mixed bool; values map[string]int64; entries map[string]int64; locked int64; truncated bool }
func (a *previewAccumulator) add(value string, entries []string, locked bool, count int64) {
 if !a.hasFirst {a.first=value; a.hasFirst=true}; if value!=a.first {a.mixed=true}
 if _,exists:=a.values[value]; exists || len(a.values)<bulkPreviewValues {a.values[value]+=count} else {a.truncated=true}
 seen:=map[string]bool{}
 for _,entry:=range entries {if seen[entry] {continue}; seen[entry]=true; if _,exists:=a.entries[entry]; exists || len(a.entries)<bulkPreviewValues {a.entries[entry]+=count} else {a.truncated=true}}
 if locked {a.locked+=count}
}
func previewCounts(counts map[string]int64) []BulkPreviewValue {
 values:=[]BulkPreviewValue{}; for value,count:=range counts {values=append(values,BulkPreviewValue{value,count})}
 sort.Slice(values,func(i,j int)bool{return values[i].Value<values[j].Value}); return values
}
// BulkPreviewTx reuses the exact editor registry, snapshot and CAS digest for
// explicit Home targets. Queries aggregate field vectors in SQLite and stream
// distinct vectors, never catalogue ids or per-entity RepairState requests.
// Output retains at most 64 values/list entries per field. Query memory is
// bounded independently of result size; explicit CAS input is capped at 2048.
func (s *Service) BulkPreviewTx(ctx context.Context, tx *sql.Tx, selection catalog.BulkMetadataSelection, authorize func(*sql.Tx) error) (BulkMetadataPreview,error) {
 out:=BulkMetadataPreview{Schema:[]RepairFieldSpec{},SingleItemFields:[]RepairFieldSpec{},ExcludedFields:[]RepairFieldSpec{},Fields:map[string]BulkPreviewField{},Targets:[]BulkPreviewTarget{},SelectionRevision:selection.Revision}
 if authorize==nil || tx==nil || strings.TrimSpace(selection.SQL)=="" || len(selection.Targets)>catalog.BulkMetadataPreviewTargetLimit {return out,ErrRepairInput}; if err:=authorize(tx); err!=nil {return out,err}
 rows,err:=tx.QueryContext(ctx,`SELECT k.name,count(*) FROM (`+selection.SQL+`) selected JOIN catalog_entities i ON i.id=selected.id JOIN catalog_kinds k ON k.id=i.kind GROUP BY k.name LIMIT 2`,selection.Args...)
 if err!=nil {return out,err}
 kinds:=0
 for rows.Next() {kinds++; if err=rows.Scan(&out.ContentKind,&out.Total); err!=nil {break}}
 if err==nil {err=rows.Err()}; rows.Close(); if err!=nil {return out,err}; if kinds!=1 {return out,ErrRepairInput}
 kind,err:=compactcatalog.ParseKind(out.ContentKind); if err!=nil {return out,err}; out.Kind=compactcatalog.OwnerNamespace(kind)
 accumulators:=map[string]*previewAccumulator{}
 explicitValues:=map[string][]string{}
 out.SingleItemFields=repairSchema(out.Kind,out.ContentKind)
 for _,spec:=range out.SingleItemFields {if spec.Bulk {out.Schema=append(out.Schema,spec); accumulators[spec.Field]=&previewAccumulator{values:map[string]int64{},entries:map[string]int64{}}} else {out.ExcludedFields=append(out.ExcludedFields,spec)}}
 if out.Kind=="item" && (out.ContentKind=="movie" || out.ContentKind=="song") {out.Schema=append(out.Schema,listSpec("genres","Genres")); accumulators["genres"]=&previewAccumulator{values:map[string]int64{},entries:map[string]int64{}}}
 if len(selection.Targets)>0 {
  for _,target:=range selection.Targets {
   repair:=RepairTarget{Kind:target.Kind,ID:target.ID}
   snapshot,entity,err:=readRepairSnapshot(ctx,tx,repair); if err!=nil {return out,err}
   revision,err:=repairRevision(ctx,tx,repair,snapshot,entity); if err!=nil {return out,err}
   if entity.library!=target.LibraryID || entity.itemKind!=out.ContentKind {return out,ErrRepairInput}
   out.Targets=append(out.Targets,BulkPreviewTarget{target.Kind,target.ID,target.LibraryID,revision})
   for _,spec:=range out.Schema {
    if spec.Field=="genres" {genres:=[]string{}; for _,relationship:=range snapshot.Relationships {if relationship.Kind=="genre" {genres=append(genres,relationship.Label)}}; genres=bulkPreviewGenres(genres); accumulators[spec.Field].add(encodeList(genres),genres,snapshot.RelationshipLocks["genre"],1); explicitValues[spec.Field]=append(explicitValues[spec.Field],encodeList(genres)); continue}
    field,ok:=snapshot.Fields[spec.Field]; if !ok {return out,ErrRepairInput}
    accumulators[spec.Field].add(field.Value,field.Values,field.Locked,1)
    explicitValues[spec.Field]=append(explicitValues[spec.Field],field.Value)
   }
  }
 } else {
  // These inputs are the same backbone/details/side values readRepairEntity
  // passes to repairFieldValue. That function and decodeList remain the sole
  // interpretation of registry values, including absent versus year zero.
  vector:=`json_object('backbone',json_object('metadata_language',i.metadata_language,'year',CAST(i.year AS TEXT)),
   'details',json_object('edition',COALESCE(d.edition,''),'content_rating',COALESCE(d.content_rating,''),'studio',COALESCE(d.studio,''),'network',COALESCE(d.network,''),'country',COALESCE(d.country,''),'tags',COALESCE(d.tags,''),'labels',COALESCE(d.labels,'')),
   'side',json(CASE k.name WHEN 'show' THEN json_object('content_rating',COALESCE(sh.content_rating,''),'studio',COALESCE(sh.studio,''),'network',COALESCE(sh.network,''),'country',COALESCE(sh.country,''),'tags',COALESCE(sh.tags,''),'labels',COALESCE(sh.labels,''))
   WHEN 'artist' THEN json_object('tags',COALESCE(ar.tags,'')) WHEN 'album' THEN json_object('label',COALESCE(al.label,''),'tags',COALESCE(al.tags,'')) WHEN 'book' THEN json_object('author',COALESCE(bk.author,''),'narrator',COALESCE(bk.narrator,''),'series',COALESCE(bk.series,''),'tags',COALESCE(bk.tags,''),'labels',COALESCE(bk.labels,'')) ELSE json_object() END),
   'locks',json(COALESCE((SELECT json_group_object(field,locked) FROM metadata_owner_fields owner WHERE owner.entity_id=i.id AND owner.kind=?),'{}')),
   'genreLocked',COALESCE((SELECT locked FROM metadata_relationship_decisions lock WHERE lock.entity_id=i.id AND lock.kind=? AND lock.relationship='genre'),0),
   'genres',json(COALESCE((SELECT json_group_array(label) FROM (SELECT DISTINCT COALESCE(ts.label_override,t.label) AS label FROM catalog_entity_terms et JOIN catalog_terms t ON t.id=et.term_id JOIN catalog_term_sources ts ON ts.entity_id=et.entity_id AND ts.term_id=et.term_id WHERE et.entity_id=i.id AND t.vocab=1 ORDER BY label)),'[]')))`
  args:=append([]any{},selection.Args...); args=append(args,out.Kind,out.Kind)
  vectors:=`WITH selected AS (`+selection.SQL+`) SELECT `+vector+` AS vector FROM selected JOIN catalog_entities i ON i.id=selected.id JOIN catalog_kinds k ON k.id=i.kind LEFT JOIN catalog_item_details d ON d.entity_id=i.id LEFT JOIN catalog_shows sh ON sh.entity_id=i.id LEFT JOIN catalog_artists ar ON ar.entity_id=i.id LEFT JOIN catalog_albums al ON al.entity_id=i.id LEFT JOIN catalog_books bk ON bk.entity_id=i.id`
  query:=`SELECT vector,count(*) FROM (`+vectors+`) GROUP BY vector`
  // Exact per-field distinct counts are separate from the bounded value list.
  for _,spec:=range out.Schema {
   var distinct int64
   bound:=append(append([]any{},args...),out.Kind,out.ContentKind,spec.Field)
   if err=tx.QueryRowContext(ctx,`WITH vectors AS (`+vectors+`) SELECT count(DISTINCT portico_bulk_metadata_value(?,?,?,vector)) FROM vectors`,bound...).Scan(&distinct); err!=nil {return out,err}
   out.Fields[spec.Field]=BulkPreviewField{DistinctCount:distinct}
  }
  grouped,err:=tx.QueryContext(ctx,query,args...); if err!=nil {return out,err}
  for grouped.Next() {
   var raw string; var count int64
   if err=grouped.Scan(&raw,&count); err!=nil {break}
   var value bulkPreviewVector
   if err=json.Unmarshal([]byte(raw),&value); err!=nil {break}
   locks:=value.Locks
   for _,spec:=range out.Schema {
    if spec.Field=="genres" {genres:=bulkPreviewGenres(value.Genres); accumulators[spec.Field].add(encodeList(genres),genres,value.GenreLocked!=0,count); continue}
    field:=repairFieldValue(out.Kind,out.ContentKind,spec.Field,value.Backbone,value.Details,value.Side,sql.NullInt64{})
    entries:=[]string{}; if spec.Type=="list" {entries=decodeList(field); field=encodeList(entries)}
    accumulators[spec.Field].add(field,entries,locks[spec.Field]!=0,count)
   }
  }
  if err==nil {err=grouped.Err()}; grouped.Close(); if err!=nil {return out,err}
 }
 for field,a:=range accumulators {
  distinct:=out.Fields[field].DistinctCount
  if len(selection.Targets)>0 {raw,err:=json.Marshal(explicitValues[field]); if err!=nil {return out,err}; if err=tx.QueryRowContext(ctx,`SELECT count(DISTINCT value) FROM json_each(?)`,string(raw)).Scan(&distinct); err!=nil {return out,err}}
  value:=BulkPreviewField{DistinctCount:distinct,State:"shared",Values:previewCounts(a.values),Entries:previewCounts(a.entries),Locked:a.locked,Truncated:a.truncated}; if a.mixed {value.State="mixed"} else {shared:=a.first; value.Value=&shared}; out.Fields[field]=value}
 if err=authorize(tx); err!=nil {return out,err}
 return out,nil
}
