package httpapi

import (
 "context"
 "database/sql"
 "net/http"
 "time"

 "portico.local/apikit"
 "portico.local/server/internal/catalog"
 "portico.local/server/internal/dbwork"
 "portico.local/server/internal/metadata"
)

// Root registers this one additive route with registerBulkMetadataPreview(r,d).
// It deliberately retains the same live owner/metadata transaction guard as
// POST metadata/bulk; the preview grants no new mutation or editor authority.
func registerBulkMetadataPreview(registry *apikit.Registry, d Dependencies) {
 v1Route(registry,apikit.Metadata{Admission:laneBrowsing,ID:"post_metadata_bulk_preview",Method:"POST",Path:"/v1/metadata/bulk/preview",Summary:"Read shared and mixed metadata for an authorized selection",Access:apikit.Owner,Requires:apikit.ScopeLibrary,Lane:apikit.Browsing,Cost:apikit.PageSized,BodyLimit:524288,Status:200,Errors:[]string{"invalid_request","unauthorized","not_permitted","not_found","revision_mismatch","internal"}},
  func(ctx context.Context,r *http.Request,in catalog.BulkMetadataPreviewRequest)(metadata.BulkMetadataPreview,error){
   ctx,cancel:=context.WithTimeout(ctx,30*time.Second); defer cancel(); r=r.WithContext(ctx)
   out:=metadata.BulkMetadataPreview{}
   if r.URL.RawQuery!="" {return out,metadata.ErrRepairInput}
   if d.Metadata==nil || d.Catalog==nil || d.Identity==nil {return out,metadata.ErrRepairInput}
   p,err:=d.manualMetadataOwner(ctx,r); if err!=nil {return out,err}
   var fence string
   err=dbwork.WithReadSnapshot(ctx,d.DB,func(snapshot context.Context)error{
    request:=r.WithContext(snapshot)
    libraries,bound,err:=d.homeScopeFor(request,p); if err!=nil {return err}; fence=bound
    viewer,err:=d.catalogViewer(request,p,libraries,bound); if err!=nil {return err}
    tx:=dbwork.Snapshot(snapshot); if tx==nil {return metadata.ErrRepairInput}
    authorize:=func(tx *sql.Tx)error{return d.manualMetadataAuthorize(snapshot,tx,p)}
    if err=authorize(tx); err!=nil {return err}
    selection,err:=d.Catalog.BulkMetadataSelectionTx(snapshot,tx,viewer,in); if err!=nil {return bulkError(err)}
    out,err=d.Metadata.BulkPreviewTx(snapshot,tx,selection,authorize)
    return err
   })
   if err!=nil {return out,bulkError(err)}
   if _,err=d.manualMetadataOwner(ctx,r); err!=nil {return metadata.BulkMetadataPreview{},err}
   libraries,bound,err:=d.homeScopeFor(r,p); _=libraries
   if err!=nil {return metadata.BulkMetadataPreview{},err}; if bound!=fence {return metadata.BulkMetadataPreview{},catalog.ErrStaleContinuation}
   out.ServerID=d.Identity.ID(); out.ViewerFence=fence
   return out,nil
  })
}
