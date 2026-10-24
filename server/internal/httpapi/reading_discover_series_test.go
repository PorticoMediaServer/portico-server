package httpapi

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "testing"

 "portico.local/server/internal/compactcatalog"
 "portico.local/server/internal/identity"
 "portico.local/server/internal/personalstate"
 "portico.local/server/internal/reading"
)

func TestReadingDiscoverSeriesCountsWholeReadyMembershipBeforePreviewLimit(t *testing.T) {
 r:=newReadingFixture(t,"book","pdf")
 ctx:=context.Background()
 var items []string
 for i:=0;i<5;i++ {
  path:=readingTreeFile(t,r,fmt.Sprintf("Author/Saga/Title %d.epub",i))
  readingScan(t,r.f,r.library)
  items=append(items,readingPathItem(t,r,path))
 }
 for i,item:=range items {
  name:="Saga";if i==0 {name="SAGA"}
  readingSeriesOwnerField(t,r,item,"series",name)
  readingSeriesOwnerField(t,r,item,"seriesNumber",fmt.Sprint(i+1))
 }
 tx,err:=r.f.db.BeginTx(ctx,nil)
 if err!=nil {t.Fatal(err)}
 for _,i:=range []int{0,4} {
  var entity int64
  if err=tx.QueryRowContext(ctx,`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`,items[i]).Scan(&entity);err!=nil {t.Fatal(err)}
  if err=personalstate.Write(tx,identity.PersonalKey(r.p.Viewer),entity,true);err!=nil {t.Fatal(err)}
 }
 if err=tx.Commit();err!=nil {t.Fatal(err)}
 if _,err=r.f.db.Exec(`UPDATE library_sources SET enabled=0 WHERE library_id=?`,r.library);err!=nil {t.Fatal(err)}
 readingSeriesDrain(t,r)
 out,err:=r.s.Discover(ctx,r.p,r.library,reading.DiscoverLimits{Recent:1,Series:1})
 if err!=nil||len(out.Series)!=1 {t.Fatalf("series composition %+v %v",out,err)}
 series:=out.Series[0]
 if series.ID!=compactcatalog.ReadingSeriesID(r.library,"saga")||series.LibraryID!=r.library||series.Name!="Saga"||series.Count!=5||series.ReadCount!=2||len(series.Preview)!=3 {t.Fatalf("exact series %+v",series)}
 for i,item:=range series.Preview {
  if item.ID!=items[i]||item.InventoryAvailable||item.Series==nil||item.Series.ID!=series.ID {t.Fatalf("canonical unavailable preview %+v",item)}
 }
 readingSeriesOwnerField(t,r,items[0],"series","Changed")
 if _,err=r.s.Discover(ctx,r.p,r.library,reading.DiscoverLimits{Series:1});!errors.Is(err,reading.ErrChanged) {t.Fatalf("unready series answered %v",err)}
 // An existing caller that did not opt into ready series keeps its recent row.
 if _,err=r.s.Discover(ctx,r.p,r.library,reading.DiscoverLimits{});err!=nil {t.Fatal(err)}
 for _,limit:=range []int{-1,21} {
  if _,err=r.s.Discover(ctx,r.p,r.library,reading.DiscoverLimits{Series:limit});!errors.Is(err,reading.ErrInvalid) {t.Fatalf("invalid limit %d: %v",limit,err)}
 }
 if _,err=r.s.Discover(ctx,r.p,"",reading.DiscoverLimits{Series:1});!errors.Is(err,reading.ErrInvalid) {t.Fatalf("mixed-library series %v",err)}
}

func TestReadingDiscoverSeriesUsesCurrentProfilePolicyAndReadState(t *testing.T) {
 r:=newReadingFixture(t,"book","pdf")
 ctx:=context.Background()
 readingSeriesOwnerField(t,r,r.item,"series","Protected series")
 readingSeriesDrain(t,r)
 owner:=readingOwnerLogin(t,r)
 profiles,err:=r.f.id.CreateDirectProfile(ctx,owner.AccountToken,"Series discover reader","blue")
 if err!=nil {t.Fatal(err)}
 var child identity.DirectProfile
 for _,profile:=range profiles.Profiles {if !profile.Primary&&profile.Name=="Series discover reader" {child=profile}}
 selected,err:=r.f.id.SelectDirectProfile(ctx,owner.AccountToken,child.ID,identity.DirectSelection{})
 if err!=nil {t.Fatal(err)}
 p,err:=r.f.id.Authenticate(selected.Session.AccessToken)
 if err!=nil {t.Fatal(err)}
 out,err:=r.s.Discover(ctx,p,r.library,reading.DiscoverLimits{Series:1})
 if err!=nil||len(out.Series)!=1||out.Series[0].ReadCount!=0 {t.Fatalf("profile series %+v %v",out,err)}
 if _,err=r.f.id.SetProfileRestrictions(ctx,owner.AccountToken,child.ID,identity.ProfileRestrictionEdit{ExpectedRevision:1,AllowUnrated:false});err!=nil {t.Fatal(err)}
 out,err=r.s.Discover(ctx,p,r.library,reading.DiscoverLimits{Series:1})
 if err!=nil||len(out.Series)!=0 {t.Fatalf("restricted series leaked %+v %v",out,err)}
 if _,err=r.f.id.EditDirectProfile(ctx,owner.AccountToken,child.ID,identity.DirectProfileEdit{ExpectedRevision:child.Revision,Policy:&identity.ProfilePolicy{AllowedLibraries:[]string{}}});err!=nil {t.Fatal(err)}
 if _,err=r.s.Discover(ctx,p,r.library,reading.DiscoverLimits{Series:1});err==nil||!errors.Is(err,sql.ErrNoRows)&&!errors.Is(err,identity.ErrForbidden) {t.Fatalf("revoked library series %v",err)}
}
