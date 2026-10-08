import {HostedAccountClient} from '@core/index.ts';
import {browserAccount} from './account';
/** Session-producing calls participate in the existing cross-tab cookie coordinator. */
export function accountSecurityClient(){
 const {api}=browserAccount();
 return new HostedAccountClient(api.origin,'browser',(path,method,body,signal,token)=>api.send(path,method,body,signal,token));
}
