import {ApiError} from '@core/index.ts';
import {AccountSessionReviewService,type AccountSessionScope} from '@core/account-sessions';
import {browserAccount} from './account';
export function accountSessionReview(scope:AccountSessionScope){
 const central=browserAccount();
 return new AccountSessionReviewService({scope,api:{async request<T>(path:string,method='GET',_body?:unknown,signal?:AbortSignal):Promise<T>{
  const session=await central.service.accessSession();if(signal?.aborted||session.account.id!==scope.accountId||session.familyId!==scope.familyId)throw new ApiError(409,'account_changed','Your Portico Account session changed. Reopen this page.');
  const response=await fetch(central.api.origin+path,{method,signal,headers:{Authorization:'Bearer '+session.accessToken}});
  const live=central.service.getSnapshot().session;if(signal?.aborted||live?.account.id!==scope.accountId||live.familyId!==scope.familyId)throw new ApiError(409,'account_changed','Your Portico Account session changed. Reopen this page.');
  if(!response.ok){const value=await response.json().catch(()=>null);throw new ApiError(response.status,value?.error?.code??'request_failed',value?.error?.message??'Could not load your account sessions.',value?.error?.retryable??response.status>=500);}
  return response.json();
 }}});
}
