import {useLayoutEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {IdentityClient, type TwoFactorEnrolment} from '@core/identity-client.ts';
import type {TwoFactorState} from '@core/identity-restrictions.ts';
import type {SecurityState} from '@core/hosted-security.ts';
import type {ProfileAPI} from '@core/profile-management.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {Button, Switch, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {SettingsGroup} from '../../../kit/settings/SettingsGroups';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import {AuthCodeBoxes} from '../../../kit/accounts';
import {HostedError, ServicesQuiet} from '../../../kit/accounts/ServicesAuth';
import {ServicesAuthenticator, ServicesRecoveryCodes} from '../../../kit/accounts/ServicesWorkspace';
import {servicesAccount, accountIdentity, currentAccountWork, sameAccount, type AccountIdentity, type ServicesAccount} from '../../../services/session';
import {HostedSecurityTask, type HostedSecurityTaskKind} from '../../../services/SecurityPanel';
import type {ProfileSettingsSource} from './source';
import {AccountPasswordFields} from './AccountPasswordFields';
import {AccountFactorProofFields} from './AccountFactorProofFields';
import {HostedSecurityAction} from './HostedSecurityAction';
import styles from './profile.module.css';

export type ProfileSecurityFacts=Readonly<{mfa?:boolean;remaining?:number;email?:string}>;
const t=defaultI18n.t;
type Task='password'|'enrol'|'disable'|'codes';
type Opening={
  live:boolean;busy:boolean;readonly abort:AbortController;readonly current:()=>boolean;
  readonly local:IdentityClient;readonly accountId:string;readonly revision:number;readonly link:string;
  hosted?:ServicesAccount;identity?:AccountIdentity;
};
type LocalIntent={readonly owner:Opening;readonly task:Task;live:boolean;enrolPassword?:string};
type HostedIntent={readonly owner:Opening;readonly kind:HostedSecurityTaskKind;readonly id:number;readonly current:()=>boolean};
const ended=()=>new DOMException('The original account security opening ended.','AbortError');

/** Sign-in belongs to the account, not the selected editable profile. The actual
 * main-profile document and original Settings lifetime gate every read and action. */
export function ProfileSignIn({profiles,api,commandApi,signIn,isCurrent,onEnded,onFacts}:{
  profiles:ProfileSettingsSource;api:ProfileAPI;commandApi:ProfileAPI;signIn:'local'|'portico';isCurrent:()=>boolean;onEnded:()=>void;onFacts?:(facts:ProfileSecurityFacts)=>void;
}){
 const document=useSyncExternalStore(profiles.subscribe,profiles.getSnapshot).document;
 const account=document?.account;
 const scope=document?.canManage&&document.capabilities.manageAccount&&account&&!account.disabled?
  JSON.stringify([account.id,account.revision,account.role,account.hostedAccountId??'',profiles.getSnapshot().inUseId]):'';
 const active=useRef<Opening|undefined>(undefined),localIntentRef=useRef<LocalIntent|undefined>(undefined),hostedIntentRef=useRef<HostedIntent|undefined>(undefined),sequence=useRef(0);
 const [opening,setOpening]=useState<Opening>(),[factor,setFactor]=useState<TwoFactorState>(),[hostedState,setHostedState]=useState<SecurityState>();
 const [task,setTask]=useState<Task>(),[hostedTask,setHostedTask]=useState<HostedIntent>();
 const [enrolment,setEnrolment]=useState<TwoFactorEnrolment>(),[codes,setCodes]=useState<readonly string[]>();
 const [password,setPassword]=useState(''),[next,setNext]=useState(''),[code,setCode]=useState('');
 const [busy,setBusy]=useState(false),[error,setError]=useState<unknown>(),[retry,setRetry]=useState(0);
 const clearSecrets=()=>{setPassword('');setNext('');setCode('');};
 useLayoutEffect(()=>{
  if(localIntentRef.current){localIntentRef.current.live=false;localIntentRef.current.enrolPassword=undefined;}localIntentRef.current=undefined;hostedIntentRef.current=undefined;
  setOpening(undefined);setFactor(undefined);setHostedState(undefined);setTask(undefined);setHostedTask(undefined);setEnrolment(undefined);setCodes(undefined);clearSecrets();setBusy(false);setError(undefined);
  if(!scope||!account||!isCurrent())return;
  const expected={accountId:account.id,revision:account.revision,role:account.role,link:account.hostedAccountId??'',profileId:profiles.getSnapshot().inUseId};
  const abort=new AbortController();
  const held:Opening={live:true,busy:false,abort,accountId:expected.accountId,revision:expected.revision,link:expected.link,
   current:()=>{
    if(!held.live||active.current!==held||abort.signal.aborted||!isCurrent())return false;
    const now=profiles.getSnapshot(),doc=now.document;
    return !!doc&&doc.canManage&&doc.capabilities.manageAccount&&!doc.account.disabled&&doc.account.id===expected.accountId&&doc.account.revision===expected.revision&&
     doc.account.role===expected.role&&(doc.account.hostedAccountId??'')===expected.link&&now.inUseId===expected.profileId;
   },local:new IdentityClient({request:(path,method,body,input)=>{
    if(!held.current())throw ended();
    return commandApi.request(path,method,body,AbortSignal.any([abort.signal,...(input?[input]:[])]));
   }})};
  active.current=held;setOpening(held);
  void(async()=>{
   if(!held.current()||(signIn==='local')===Boolean(expected.link))throw ended();
   if(signIn==='local'){
    const value=await new IdentityClient(api).twoFactor(abort.signal);
    if(held.current())setFactor(value);
   }else{
    // This branch alone touches Hosted. A local account performs zero Hosted IO.
    const hosted=servicesAccount();held.hosted=hosted;
    const result=await currentAccountWork(hosted,async session=>{
     if(!held.current()||session.account.id!==expected.link)throw ended();
     const identity=Object.freeze(accountIdentity(session));
     const state=await hosted.client.state(identity,session.accessToken,abort.signal);
     if(!held.current())throw ended();
     return {identity,state};
    });
    if(!held.current())return;
    held.identity=result.identity;setHostedState(result.state);
   }
  })().catch(failure=>{if(held.current())setError(failure);});
  return()=>{held.live=false;const intent=localIntentRef.current;if(intent?.owner===held){intent.live=false;intent.enrolPassword=undefined;localIntentRef.current=undefined;}if(hostedIntentRef.current?.owner===held)hostedIntentRef.current=undefined;abort.abort();if(active.current===held)active.current=undefined;};
 },[api,commandApi,profiles,scope,signIn,isCurrent,retry]);
 const held=opening&&active.current===opening&&opening.current()?opening:undefined;
 const hostedCurrent=()=>{
  const identity=held?.identity,current=held?.hosted?.service.getSnapshot().session;
  return !!held&&held.current()&&!!identity&&!!current&&sameAccount(identity,accountIdentity(current))&&current.account.id===held.link;
 };
 const intent=localIntentRef.current?.owner===held?localIntentRef.current:undefined;
 const intentCurrent=(value:LocalIntent|undefined)=>!!held&&!!value&&value.live&&value.owner===held&&localIntentRef.current===value&&held.current();
 const close=()=>{if(!held||held.busy||!intentCurrent(intent))return;intent!.live=false;intent!.enrolPassword=undefined;localIntentRef.current=undefined;setTask(undefined);setEnrolment(undefined);setCodes(undefined);clearSecrets();setError(undefined);};
 const openHosted=(kind:HostedSecurityTaskKind)=>{
  if(!held||held.busy||localIntentRef.current||hostedIntentRef.current||!hostedCurrent()||!hostedState?.hasPassword)return;
  const next:HostedIntent={owner:held,kind,id:++sequence.current,current:()=>hostedIntentRef.current===next&&held.current()};
  hostedIntentRef.current=next;setHostedTask(next);
 };
 const open=(value:Task)=>{
  if(!held||held.busy||localIntentRef.current||hostedIntentRef.current||codes)return;
  clearSecrets();setError(undefined);
  if(signIn==='portico')openHosted(value==='password'?'password_set_or_change':value==='enrol'?'mfa_enroll':value==='disable'?'mfa_disable':'recovery_codes');
  else{localIntentRef.current={owner:held,task:value,live:true};setTask(value);}
 };
 const submit=async()=>{
  if(!held||!intent||!intentCurrent(intent)||held.busy||!task||intent.task!==task)return;
  const ownedTask=task,proof=password,newPassword=next,submittedCode=code,prior=enrolment;
  if(ownedTask==='password'&&(!proof||!passwordStrength(newPassword).acceptable||factor?.enabled&&!submittedCode.trim()))return;
  if(ownedTask==='enrol'?(prior?!intent.enrolPassword||submittedCode.length!==6:!proof):!proof||ownedTask!=='password'&&!submittedCode.trim())return;
  held.busy=true;setBusy(true);setError(undefined);clearSecrets();
  try{
   if(!intentCurrent(intent))throw ended();
   if(ownedTask==='password'){
    await held.local.changePassword(proof,newPassword,submittedCode);
    // A known terminal acknowledgement retires the original shell, never a successor.
    onEnded();return;
   }
   if(ownedTask==='disable'){
    await held.local.disableTwoFactor(proof,submittedCode);onEnded();return;
   }
   if(ownedTask==='codes'){
    const fresh=await held.local.replaceRecoveryCodes(proof,submittedCode,held.abort.signal);
    if(!intentCurrent(intent))return;setCodes(fresh);setTask(undefined);
   }else if(!prior){
    const fresh=await held.local.enrolTwoFactor(proof);
    if(!intentCurrent(intent))return;intent.enrolPassword=proof;setEnrolment(fresh);
   }else{
    const originalPassword=intent.enrolPassword!;intent.enrolPassword=undefined;
    const fresh=await held.local.verifyTwoFactor(originalPassword,submittedCode);
    if(!intentCurrent(intent))return;setFactor(fresh);setCodes(prior.recoveryCodes);setTask(undefined);setEnrolment(undefined);
   }
  }catch(failure){if(intentCurrent(intent)){if(ownedTask==='enrol'&&prior){intent.enrolPassword=undefined;setEnrolment(undefined);}setError(failure);}}
  finally{held.busy=false;if(held.current())setBusy(false);}
 };
 const mfa=signIn==='local'?factor?.enabled:hostedState?.mfaEnabled;
 const remaining=signIn==='local'?factor?.recoveryCodesRemaining:hostedState?.recoveryCodesRemaining;
 useLayoutEffect(()=>{if(held?.current())onFacts?.({mfa,remaining,email:hostedState?.contactAddress});},[held,mfa,remaining,hostedState?.contactAddress,onFacts]);
 const available=!!held&&(signIn==='local'?!!factor:!!hostedState&&hostedCurrent()&&hostedState.hasPassword);
 const guardField=(change:(value:string)=>void)=>(value:string)=>{if(intentCurrent(intent)&&!held!.busy)change(value);};
 const ready=!!held&&!busy&&(task==='password'?!!password&&passwordStrength(next).acceptable&&(!factor?.enabled||!!code.trim()):task==='enrol'?enrolment?!!intent?.enrolPassword&&code.length===6:!!password:!!password&&!!code.trim());
 const title=t(task==='password'?'account.changePasswordTitle':task==='disable'?'web.twoStep.turnOffConfirmTitle':task==='codes'||codes?'web.hostedSecurity.newCodesTitle':enrolment?'web.hostedSecurity.addToAuthenticator':'web.twoStep.turnOnTitle');
 if(!scope||!document?.canManage)return null;
 return <SettingsGroup title={t('next.settingsSpec.account.profile.groups.5.title')}>
  {error&&!task&&!hostedTask&&!codes?<HostedError error={error}/>:null}
  {error&&!task&&!hostedTask&&<Button size="small" onClick={()=>{if(held?.current())setRetry(value=>value+1);}}>{t('next.states.tryAgain')}</Button>}
  {signIn==='portico'&&<SettingsRow id="profile.email" label={t('next.settingsSpec.account.profile.groups.5.rows.0.label')} help={<>{hostedState?.contactAddress}<br/>{t('services.security.emailHelp')}</>}>
   <HostedSecurityAction label={t('next.settingsSpec.account.profile.groups.5.rows.0.control.label')} disabled={!available||busy} onOpen={()=>{openHosted('email_change');}}/>
  </SettingsRow>}
  <SettingsRow id="profile.password" label={t('next.settingsSpec.account.profile.groups.5.rows.1.label')}>
   <HostedSecurityAction label={t('next.settingsSpec.account.profile.groups.5.rows.1.control.label')} disabled={!available||busy} onOpen={()=>open('password')}/>
  </SettingsRow>
  <SettingsRow id="profile.twoStep" label={t('next.settingsSpec.account.profile.groups.5.rows.2.label')} help={t('next.settingsSpec.account.profile.groups.5.rows.2.help')}>
   {mfa===undefined?<span role="status">{t('web.claim.loading')}</span>:<Switch aria-label={t('next.settingsSpec.account.profile.groups.5.rows.2.label')} checked={mfa===true} disabled={!available||busy} onCheckedChange={()=>open(mfa?'disable':'enrol')}/>}
  </SettingsRow>
  {mfa&&<SettingsRow id="profile.recoveryCodes" dependent label={t('next.settingsSpec.account.profile.groups.5.rows.2.children.0.label')} help={remaining===undefined?undefined:t('web.hostedSecurity.codesLeft',{count:remaining})}>
   <HostedSecurityAction label={t('next.settingsSpec.account.profile.groups.5.rows.2.children.0.control.label')} disabled={!available||busy} onOpen={()=>open('codes')}/>
  </SettingsRow>}
  {hostedState&&!hostedState.hasPassword&&<ServicesQuiet>{t('web.hostedSecurity.providerOnly')}</ServicesQuiet>}
  {held&&held.hosted&&held.identity&&hostedState&&hostedTask&&hostedTask.current()&&<HostedSecurityTask key={hostedTask.id} account={held.hosted} identity={held.identity} state={hostedState} task={hostedTask.kind} isCurrent={hostedTask.current}
   onClose={()=>{if(hostedTask.current()){hostedIntentRef.current=undefined;setHostedTask(undefined);setRetry(value=>value+1);}}}
   onSecurityEnded={values=>{if(hostedTask.current()){
    hostedIntentRef.current=undefined;setHostedTask(undefined);setHostedState(undefined);
    if(values){localIntentRef.current={owner:held,task:'codes',live:true};setCodes(Object.freeze([...values]));}else setRetry(value=>value+1);
   }}}/>
  }
  {held&&intentCurrent(intent)&&(task||codes)&&<Dialog open title={title} showCloseButton={false} onOpenChange={value=>{if(!value)close();}}
   footer={codes?<Button variant="primary" onClick={close}>{t('action.done')}</Button>:<><Button disabled={busy} onClick={close}>{t('action.cancel')}</Button><Button variant="primary" disabled={!ready} loading={busy} onClick={()=>void submit()}>{t(task==='password'?'account.changePasswordTitle':'action.continue')}</Button></>}>
   {error?<HostedError error={error}/>:null}
   {codes?<><ServicesQuiet>{t('services.security.codesBody')}</ServicesQuiet><ServicesRecoveryCodes codes={codes}
    onCopy={()=>{if(intentCurrent(intent))void navigator.clipboard.writeText(codes.join('\n')).catch(failure=>{if(held.current())setError(failure);});}}
    onDownload={()=>{if(!intentCurrent(intent))return;let url:string|undefined;try{url=URL.createObjectURL(new Blob([codes.join('\n')],{type:'text/plain;charset=utf-8'}));const link=globalThis.document.createElement('a');link.href=url;link.download='portico-recovery-codes.txt';if(intentCurrent(intent))link.click();}catch(failure){if(held.current())setError(failure);}finally{if(url)URL.revokeObjectURL(url);}}}/></>:
    task==='password'?<><ServicesQuiet>{t('web.hostedSecurity.othersSignedOut')}</ServicesQuiet><AccountPasswordFields currentPassword={password} newPassword={next} code={code} twoFactor={factor?.enabled===true} disabled={busy}
     onCurrentPasswordChange={guardField(setPassword)} onNewPasswordChange={guardField(setNext)} onCodeChange={guardField(setCode)}/></>:
    task==='enrol'?<div className={styles.fields}>{enrolment?<><ServicesQuiet>{t('web.hostedSecurity.enterKey')}</ServicesQuiet><ServicesAuthenticator secret={enrolment.secret} uri={enrolment.uri} onCopy={()=>{if(intentCurrent(intent))void navigator.clipboard.writeText(enrolment.secret).catch(failure=>{if(held.current())setError(failure);});}} onError={failure=>{if(held.current())setError(failure);}}/>
     <AuthCodeBoxes length={6} value={code} onChange={guardField(setCode)} onComplete={()=>{}} label={t('web.twoStep.code')} disabled={busy}/></>:
     <TextField type="password" autoComplete="current-password" label={t('account.currentPassword')} value={password} onChange={event=>guardField(setPassword)(event.currentTarget.value)} disabled={busy}/>}</div>:
    <><ServicesQuiet>{t(task==='disable'?'services.security.disableBody':'next.settingsSpec.account.profile.groups.5.rows.2.children.0.control.confirm')}</ServicesQuiet><AccountFactorProofFields password={password} code={code} disabled={busy} onPasswordChange={guardField(setPassword)} onCodeChange={guardField(setCode)}/></>}
  </Dialog>}
 </SettingsGroup>;
}
