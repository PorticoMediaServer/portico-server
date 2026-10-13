package httpapi

import (
 "net/http"
 "strings"

 "portico.local/apikit"
 "portico.local/server/internal/identity"
 "portico.local/server/internal/social"
)

// Social handlers keep their bounded protocol envelopes while the registry owns
// their routes, authorization and generated request/response contracts.
func socialContract[Req,Resp any](r *apikit.Registry,pattern string,h http.HandlerFunc,status int){
 parts:=strings.SplitN(pattern," ",2)
 lane:=apikit.Default
 if parts[0]=="GET"{lane=apikit.Browsing}
 m:=apikit.Metadata{ID:"social_"+strings.NewReplacer("/","_","{","","}","","-","_").Replace(strings.ToLower(parts[0])+parts[1]),Method:parts[0],Path:parts[1],Summary:"Manage shared playback for the signed-in profile",Access:apikit.Device,Lane:lane,Cost:apikit.PageSized,BodyLimit:256<<10,Status:status,Errors:[]string{"invalid_request","unauthorized","not_permitted","not_found","rate_limited","internal"}}
 if e:=apikit.RegisterRaw(r,apikit.RawRoute[Req,Resp]{Metadata:m,Serve:h});e!=nil{panic(e)}
}

var socialRegistered=map[string]bool{"POST /v1/groups":true,"GET /v1/groups":true,"POST /v1/groups/join":true,"GET /v1/groups/{id}":true,"POST /v1/groups/{id}/heartbeat":true,"POST /v1/groups/{id}/leave":true,"POST /v1/groups/{id}/end":true,"POST /v1/groups/{id}/host-transfer":true,"POST /v1/groups/{id}/transport":true,"POST /v1/groups/{id}/settings":true,"POST /v1/groups/{id}/readiness":true,"GET /v1/groups/{id}/queue":true,"POST /v1/groups/{id}/queue":true,"POST /v1/receivers":true,"POST /v1/receivers/{id}/grants":true,"POST /v1/receivers/{id}/grants/{grantId}/decision":true,"POST /v1/handoffs":true,"POST /v1/handoffs/{id}/readiness":true,"POST /v1/handoffs/{id}/commit":true}

func registerSocial(r *apikit.Registry,d Dependencies){
 store:=d.Social()
 viewer:=func(w http.ResponseWriter,q *http.Request)(identity.Principal,bool){p,e:=d.principal(q);if e!=nil{socialFailure(w,e);return p,false};return p,true}
 handle:=func(pattern string,h http.HandlerFunc){
  switch pattern {
  case "POST /v1/groups":socialContract[social.CreateGroupRequest,social.GroupResponse](r,pattern,h,201)
  case "GET /v1/groups":socialContract[noBody,social.GroupListResponse](r,pattern,h,200)
  case "POST /v1/groups/join":socialContract[social.JoinRequest,social.GroupResponse](r,pattern,h,200)
  case "GET /v1/groups/{id}":socialContract[noBody,social.GroupResponse](r,pattern,h,200)
  case "POST /v1/groups/{id}/heartbeat", "POST /v1/groups/{id}/leave":socialContract[noBody,social.GroupResponse](r,pattern,h,200)
  case "POST /v1/groups/{id}/end":socialContract[social.EndRequest,social.GroupResponse](r,pattern,h,200)
  case "POST /v1/groups/{id}/host-transfer":socialContract[social.TransferRequest,social.GroupResponse](r,pattern,h,200)
  case "POST /v1/groups/{id}/transport":socialContract[social.TransportRequest,social.TransportReceipt](r,pattern,h,200)
  case "POST /v1/groups/{id}/settings":socialContract[social.SettingsRequest,social.GroupResponse](r,pattern,h,200)
  case "POST /v1/groups/{id}/readiness":socialContract[social.ReadinessRequest,social.GroupResponse](r,pattern,h,200)
  case "GET /v1/groups/{id}/queue":socialContract[noBody,social.QueueResponse](r,pattern,h,200)
  case "POST /v1/groups/{id}/queue":socialContract[social.QueueRequest,social.QueueResponse](r,pattern,h,200)
  case "POST /v1/receivers":socialContract[social.ReceiverRequest,social.ReceiverResponse](r,pattern,h,201)
  case "POST /v1/receivers/{id}/grants":socialContract[social.GrantRequest,social.GrantResponse](r,pattern,h,201)
  case "POST /v1/receivers/{id}/grants/{grantId}/decision":socialContract[social.GrantDecision,social.GrantResponse](r,pattern,h,200)
  case "POST /v1/handoffs":socialContract[social.HandoffRequest,social.HandoffResponse](r,pattern,h,201)
  case "POST /v1/handoffs/{id}/readiness":socialContract[social.HandoffReadiness,social.HandoffResponse](r,pattern,h,200)
  case "POST /v1/handoffs/{id}/commit":socialContract[social.HandoffCommit,social.HandoffResponse](r,pattern,h,200)
  }
 }
 d.groupRoutes(nil,handle,viewer,store)
 d.receiverRoutes(nil,handle,viewer,store)
}
