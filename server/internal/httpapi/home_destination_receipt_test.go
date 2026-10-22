package httpapi

import (
 "encoding/json"
 "testing"

 "portico.local/server/internal/catalog"
)

func TestHomeDestinationReceiptHTTPRefusesForeignJobSetsAndUnboundedReads(t *testing.T) {
 handler,owner,_:=editorHTTPFixture(t)
 body,_:=json.Marshal(catalog.HomeDestinationReceiptRequest{Kind:"playlist",DestinationID:"unknown",JobIDs:[]string{"foreign-job"}})
 response:=editorRequest(t,handler,"POST","/v1/home/destination-receipt",owner,"application/json",body)
 if response.Code==200 || response.Code==404 && response.Body.String()=="404 page not found\n" {t.Fatalf("receipt route missing or foreign jobs disclosed: %d %s",response.Code,response.Body.String())}
 anonymous:=editorRequest(t,handler,"POST","/v1/home/destination-receipt","","application/json",body)
 if anonymous.Code==200 {t.Fatal("anonymous receipt read accepted")}
 ids:=make([]string,201); for index:=range ids {ids[index]="job"}
 invalid,_:=json.Marshal(catalog.HomeDestinationReceiptRequest{Kind:"playlist",DestinationID:"unknown",JobIDs:ids})
 oversized:=editorRequest(t,handler,"POST","/v1/home/destination-receipt",owner,"application/json",invalid)
 if oversized.Code==200 {t.Fatal("unbounded receipt read accepted")}
}
