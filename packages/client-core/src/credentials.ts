import {randomId} from './random-id.ts';
import { hostedGate, type HostedGate } from "./hosted-gate.ts";
import { CodedError } from "./server-messages.ts";
import {
  ApiError,
  HttpLocalApi,
  type HostedSession,
  type HostedServer,
} from "./index.ts";
export type CredentialContext = { profileId?: string; server?: HostedServer };
export type CredentialRecord<Authority = string> = {
  version: 1;
  active?: {
    session: HostedSession;
    authority: Authority;
    context: CredentialContext;
    pendingRequestId?: string;
  };
  revocations: Authority[];
};
export interface CredentialStorage<Authority = string> {
  load(): Promise<CredentialRecord<Authority> | undefined>;
  save(value: CredentialRecord<Authority>): Promise<void>;
}
export interface CredentialApi<Authority = string> {
  authorityFromSession(session: HostedSession): Authority;
  refresh(
    authority: Authority,
    requestId: string,
    signal?: AbortSignal
  ): Promise<HostedSession>;
  revoke(authority: Authority, signal?: AbortSignal): Promise<void>;
  /** Shared circuit for this account service; automatic work defers to it. */
  gate?: HostedGate;
}
export class HttpCredentialApi implements CredentialApi {
  private http: HttpLocalApi;
  readonly gate: HostedGate;
  constructor(origin: string, fetcher?: typeof fetch) {
    this.http = new HttpLocalApi(origin, "", fetcher);
    this.gate = hostedGate(origin);
  }
  authorityFromSession(session: HostedSession) {
    if (!session.refreshToken)
      throw new Error("Native refresh credentials missing.");
    return session.refreshToken;
  }
  refresh(refreshToken: string, requestId: string, signal?: AbortSignal) {
    return this.gate.run("interactive", () => this.http.request<HostedSession>(
      "/v1/sessions/refresh",
      "POST",
      { refreshToken, requestId },
      signal
    ));
  }
  revoke(refreshToken: string, signal?: AbortSignal) {
    return this.gate.run("interactive", () => this.http.request<void>(
      "/v1/sessions/revoke",
      "POST",
      { refreshToken },
      signal
    ));
  }
}
/** Renew once less than this much of the 90-day sign-in window remains (a third used). */
export const KEEP_ALIVE_REMAINING_MS = 60 * 86400000;
export interface CredentialCoordinator { runExclusive<T>(operation:()=>Promise<T>):Promise<T> }
export type CredentialSnapshot = {
  phase: "restoring" | "signedOut" | "ready" | "refreshing" | "error";
  session?: Omit<HostedSession, "refreshToken">;
  context?: CredentialContext;
  error?: string;
  pendingRevocations: number;
};
/** Owns refresh credentials; renderer snapshots never contain refresh tokens. */
/** Hosted refusals that mean this browser's refresh credential can never renew. */
const unusableCredentialCodes = new Set(["csrf_required", "origin_denied", "session_mode_denied"]);

export class CredentialSessionService<Authority = string> {
  private api: CredentialApi<Authority>;
  private storage: CredentialStorage<Authority>;
  private requestId: () => Promise<string>;
  private timeout: number;
  private coordinator: CredentialCoordinator;
  private record: CredentialRecord<Authority> = { version: 1, revocations: [] };
  private state: CredentialSnapshot = {
    phase: "restoring",
    pendingRevocations: 0,
  };
  private listeners = new Set<() => void>();
  private generation = 0;
  private ready: Promise<void>;
  private storageTail = Promise.resolve();
  private flight?: {
    generation: number;
    promise: Promise<Omit<HostedSession, "refreshToken">>;
  };
  private revoking = new Set<string>();
  constructor(
    api: CredentialApi<Authority>,
    storage: CredentialStorage<Authority>,
    requestId: () => Promise<string> = async () =>
      randomId(),
    timeout = 15000,
    coordinator?: CredentialCoordinator
  ) {
    this.api = api;
    this.storage = storage;
    this.requestId = requestId;
    this.timeout = timeout;
    let operationTail=Promise.resolve();
    this.coordinator=coordinator??{runExclusive:<T>(operation:()=>Promise<T>)=>{const result=operationTail.then(operation);operationTail=result.then(()=>{},()=>{});return result;}};
    this.ready = storage
      .load()
      .then((value) => {
        if (value) {
          if (value.version !== 1 || !Array.isArray(value.revocations))
            throw new Error("Saved credentials are invalid.");
          if(this.generation===0)this.record=value;
          else { this.record.revocations.push(...value.revocations);if(value.active)this.addRevocation(value.active.authority); }
        }
        this.publish();

      })
      .catch((e) => {
        this.state = {
          phase: "error",
          pendingRevocations: 0,
          error:
            e instanceof Error ? e.message : "Secure storage is unavailable.",
        };
        this.emit();
        throw e;
      });
    void this.ready.catch(() => {});
  }
  getSnapshot = () => this.state;
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  private emit() {
    this.listeners.forEach((fn) => fn());
  }
  private publicSession(session: HostedSession) {
    const { refreshToken: _, ...visible } = session;
    return visible;
  }
  private publish(phase?: CredentialSnapshot["phase"], error?: string) {
    const active = this.record.active;
    this.state = {
      phase: phase ?? (active ? "ready" : "signedOut"),
      session: active ? this.publicSession(active.session) : undefined,
      context: active?.context,
      error,
      pendingRevocations: this.record.revocations.length,
    };
    this.emit();
  }
  private persist() {
    const snapshot = JSON.parse(
      JSON.stringify(this.record)
    ) as CredentialRecord<Authority>;
    // Persist selection metadata only. Directory verification receipts are live
    // discovery evidence, not credentials or durable endpoint authority.
    if (snapshot.active?.context.server) {
      const {id,name,baseUrl,publicKey,policyRevision}=snapshot.active.context.server;
      snapshot.active.context.server={id,name,baseUrl,publicKey,policyRevision};
    }
    const result = this.storageTail.then(() => this.storage.save(snapshot));
    this.storageTail = result.catch(() => {});
    return result;
  }
  private async bounded<T>(fn: (signal: AbortSignal) => Promise<T>) {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    try {
      return await Promise.race([
        fn(controller.signal),
        new Promise<never>((_, reject) => {
          timer = setTimeout(() => {
            controller.abort();
            reject(new Error("The account service timed out."));
          }, this.timeout);
        }),
      ]);
    } finally {
      clearTimeout(timer!);
    }
  }
  private async reload(publish=true) {
    const loaded=await this.storage.load()??{version:1 as const,revocations:[]};
    if(loaded.version!==1||!Array.isArray(loaded.revocations))throw new Error("Saved credentials are invalid.");
    const current=this.record.active;
    if(loaded.active && current && !loaded.active.session.accessToken && loaded.active.session.familyId===current.session.familyId && loaded.active.session.expiresAt===current.session.expiresAt && loaded.active.pendingRequestId===current.pendingRequestId)loaded.active.session={...loaded.active.session,accessToken:current.session.accessToken};
    this.record=loaded;if(publish)this.publish();
  }
  async synchronize(){const generation=this.generation;await this.ready;return this.coordinator.runExclusive(async()=>{await this.reload(false);if(generation===this.generation)this.publish()});}
  async authenticate(callback:(signal:AbortSignal)=>Promise<HostedSession>,context:CredentialContext={}){
    const generation=++this.generation;await this.ready;
    return this.coordinator.runExclusive(async()=>{
      await this.reload(false);if(generation!==this.generation)throw new CodedError('cancelled',"Sign-in was cancelled.");
      if(this.record.active){await this.logoutInside(false);if(this.record.revocations.length)throw new CodedError('account_signout_pending',"Reconnect to finish signing out before switching accounts.");}
      if(generation!==this.generation)throw new CodedError('cancelled',"Sign-in was cancelled.");
      let session:HostedSession;
      try{session=await this.bounded(signal=>callback(signal).then(value=>{if(generation!==this.generation)void this.compensate(this.api.authorityFromSession(value));return value}));}
      catch(e){if(generation===this.generation)++this.generation;throw e;}
      if(generation!==this.generation)throw new CodedError('cancelled',"Sign-in was cancelled.");
      await this.adoptInside(session,context);return this.publicSession(session);
    });
  }
  /** Automatic approval polling cannot replace a family adopted in another tab. */
  async authenticateIfSignedOut(callback:(signal:AbortSignal)=>Promise<HostedSession>,context:CredentialContext={}) {
    await this.ready;
    return this.coordinator.runExclusive(async()=>{
      await this.reload(false);
      if(this.record.active)throw new ApiError(409,"account_changed","Another account session is already active. Start approval again after signing out.",false);
      if(this.record.revocations.length){await this.retryInside();if(this.record.revocations.length)throw new ApiError(409,"revocation_pending","Reconnect to finish signing out before approving another account.",false);}
      const generation=++this.generation;
      let session:HostedSession;
      try{session=await this.bounded(signal=>callback(signal).then(value=>{if(generation!==this.generation)void this.compensate(this.api.authorityFromSession(value));return value;}));}
      catch(e){if(generation===this.generation)++this.generation;throw e;}
      if(generation!==this.generation)throw new CodedError('cancelled',"Sign-in was cancelled.");
      await this.adoptInside(session,context);return this.publicSession(session);
    });
  }
  async adopt(session:HostedSession,context:CredentialContext={}){const generation=++this.generation;await this.ready;return this.coordinator.runExclusive(async()=>{await this.reload(false);if(generation!==this.generation)throw new CodedError('cancelled',"Sign-in was cancelled.");await this.adoptInside(session,context)});}
  private async adoptInside(session: HostedSession, context: CredentialContext = {}) {
    if (!session.familyId || !session.refreshExpiresAt)
      throw new Error(
        "This account service does not support secure session renewal."
      );
    if (
      !Number.isFinite(Date.parse(session.expiresAt)) ||
      !Number.isFinite(Date.parse(session.refreshExpiresAt))
    )
      throw new CodedError('invalid_response',"The account session expiry is invalid.");
    if (this.record.revocations.length >= 16) {
      await this.retryInside();
      if (this.record.revocations.length >= 16)
        throw new CodedError('account_signout_pending',"Reconnect to finish signing out previous sessions.");
    }
    const authority = this.api.authorityFromSession(session);
    const generation = ++this.generation;
    const old = this.record.active;
    const previous = this.record;
    this.record = {
      version: 1,
      active: { session, authority, context },
      revocations: [...this.record.revocations],
    };
    if (old && old.session.familyId !== session.familyId)
      this.addRevocation(old.authority);
    try {
      await this.persist();
    } catch (e) {
      if (generation === this.generation) this.record = previous;
      this.addRevocation(authority);
      try { await this.persist(); } catch {}
      await this.retryInside();
      throw e;
    }
    if (generation !== this.generation)
      throw new CodedError('cancelled',"Sign-in was cancelled.");
    this.publish();
    await this.retryInside();
  }
  async updateContext(context:CredentialContext){const generation=this.generation;await this.ready;return this.coordinator.runExclusive(async()=>{await this.reload(false);if(generation!==this.generation||!this.record.active)return;this.record.active={...this.record.active,context};await this.persist();if(generation===this.generation)this.publish()});}
  /** Refresh display metadata without replacing account authority or selection. */
  async refreshProfiles(
    expected: { accountId: string; familyId: string },
    load: (session: Omit<HostedSession, "refreshToken">, signal: AbortSignal) => Promise<{ accountId: string; profiles: readonly { id: string; name: string }[] }>,
  ): Promise<void> {
    const generation = this.generation;
    await this.ready;
    await this.coordinator.runExclusive(async () => {
      await this.reload(false);
      const current = () => {
        const active = this.record.active;
        if (generation !== this.generation || !active || active.session.account.id !== expected.accountId || active.session.familyId !== expected.familyId)
          throw new Error("Your Portico Account changed. Refresh your profiles.");
        return active;
      };
      let active = current();
      if (!active.session.accessToken || active.pendingRequestId || Date.parse(active.session.expiresAt) <= Date.now() + 60000) {
        await this.rotate(generation);
        active = current();
      }
      const result = await this.bounded(signal => load(this.publicSession(active.session), signal));
      active = current();
      if (!result || result.accountId !== expected.accountId || !Array.isArray(result.profiles) || result.profiles.length > 100)
        throw new Error("The profile response is invalid.");
      const ids = new Set<string>();
      const profiles = result.profiles.map(profile => {
        if (!profile || typeof profile.id !== "string" || !profile.id || profile.id.length > 256 || /[\x00-\x1f\x7f]/.test(profile.id) || ids.has(profile.id) || typeof profile.name !== "string" || !profile.name.trim() || new TextEncoder().encode(profile.name).length > 120 || /[\x00-\x1f\x7f]/.test(profile.name))
          throw new Error("The profile response is invalid.");
        ids.add(profile.id);
        return { id: profile.id, name: profile.name };
      });
      this.record.active = { ...active, session: { ...active.session, profiles } };
      try { await this.persist(); }
      catch (error) {
        if (generation === this.generation) this.record.active = active;
        throw error;
      }
      current();
      this.publish();
    });
  }
  async accessSession(forceRefresh=false):Promise<Omit<HostedSession,"refreshToken">>{
    await this.ready;
    if(this.flight?.generation===this.generation)return this.flight.promise;
    const generation=this.generation;
    const promise=this.coordinator.runExclusive(async()=>{
      if(generation!==this.generation)throw new CodedError('account_session_changed',"Session changed.");
      await this.reload(false);if(generation!==this.generation)throw new CodedError('account_session_changed',"Session changed.");this.publish();const active=this.record.active;
      if(!active)throw new CodedError('account_signed_out',"Sign in with your Portico Account.");
      if(!forceRefresh&&!active.pendingRequestId&&active.session.accessToken&&Date.parse(active.session.expiresAt)>Date.now()+60000)return this.publicSession(active.session);
      return this.rotate(this.generation);
    });
    this.flight={generation,promise};try{return await promise}finally{if(this.flight?.promise===promise)this.flight=undefined}
  }
  private async rotate(
    generation: number
  ): Promise<Omit<HostedSession, "refreshToken">> {
    const active = this.record.active!;
    const authority = active.authority;
    if (!active.pendingRequestId) {
      const requestId = await this.requestId();
      if (generation !== this.generation) throw new CodedError('account_session_changed',"Session changed.");
      this.record.active = {
        ...this.record.active!,
        pendingRequestId: requestId,
      };
    }
    const pending = this.record.active!;
    await this.persist();
    if (generation !== this.generation) throw new CodedError('account_session_changed',"Session changed.");
    this.publish("refreshing");
    try {
      const session = await this.bounded((signal) =>
        this.api
          .refresh(authority, pending.pendingRequestId!, signal)
          .then((value) => {
            if (generation !== this.generation)
              void this.compensate(this.api.authorityFromSession(value));
            return value;
          })
      );
      if (generation !== this.generation) throw new CodedError('account_session_changed',"Session changed.");
      if (
        session.familyId !== pending.session.familyId ||
        session.account.id !== pending.session.account.id
      )
        throw new CodedError('invalid_response',"The renewed account session is invalid.");
      this.record.active = {
        session,
        authority: this.api.authorityFromSession(session),
        context: this.record.active!.context,
      };
      try {
        await this.persist();
      } catch (e) {
        if (generation === this.generation) this.record.active = pending;
        throw e;
      }
      if (generation !== this.generation) throw new CodedError('account_session_changed',"Session changed.");
      this.publish();
      return this.publicSession(session);
    } catch (e) {
      if (generation === this.generation) {
        // An ended session (401, a changed family) and a browser credential Hosted refuses
        // outright (a CSRF, origin or mode mismatch: only a fresh sign-in issues a usable
        // cookie pair) both end this sign-in, so the app shows the sign-in form again.
        if (e instanceof ApiError && (e.status === 401 || (e.status === 409 && e.code === "family_changed") || (e.status === 403 && unusableCredentialCodes.has(e.code)))) {
          await this.logoutInside();
          this.publish("signedOut", e.message);
        } else
          this.publish(
            "error",
            e instanceof Error ? e.message : "Account renewal failed."
          );
      }
      throw e;
    }
  }
  private addRevocation(authority: Authority) {
    if (
      !this.record.revocations.some(
        (a) => JSON.stringify(a) === JSON.stringify(authority)
      )
    )
      this.record.revocations.push(authority);
  }
  private async compensate(authority:Authority){
    await this.coordinator.runExclusive(async()=>{await this.reload(false);this.addRevocation(authority);try{await this.persist();await this.retryInside()}catch{this.publish(this.record.active?"error":"signedOut","Session revocation is waiting for secure storage.")}});
  }
  async logout(){
    ++this.generation;const authority=this.record.active?.authority;this.record.active=undefined;this.publish("signedOut");
    await this.ready;
    return this.coordinator.runExclusive(async()=>{await this.reload(false);if(authority!==undefined)this.addRevocation(authority);await this.logoutInside()});
  }
  /** End the reviewed family without clearing an account adopted while cleanup waits. */
  async logoutIfCurrent(expected: { accountId: string; familyId: string }): Promise<boolean> {
    const scope = { accountId: expected.accountId, familyId: expected.familyId };
    const matches = (active: CredentialRecord<Authority>["active"]) =>
      !!active && active.session.account.id === scope.accountId && active.session.familyId === scope.familyId;
    const active = this.record.active;
    if (!matches(active)) return false;
    const authority = active!.authority;
    ++this.generation;
    this.record.active = undefined;
    this.publish("signedOut");
    await this.ready;
    return this.coordinator.runExclusive(async () => {
      await this.reload(false);
      this.addRevocation(authority);
      // A queued adoption may already have committed. Never revoke that family,
      // or advance its generation and cancel an adoption still in the queue.
      if (matches(this.record.active)) this.record.active = undefined;
      try {
        await this.persist();
      } catch (e) {
        this.publish(this.record.active ? "error" : "signedOut", "Sign-out could not be saved. Retry before closing the app.");
        throw e;
      }
      this.publish();
      await this.retryInside();
      return true;
    });
  }
  private async logoutInside(advanceGeneration=true){
    const authority=this.record.active?.authority;if(advanceGeneration)++this.generation;this.record.active=undefined;if(authority!==undefined)this.addRevocation(authority);this.publish("signedOut");
    try{await this.persist()}catch(e){this.publish("signedOut","Sign-out could not be saved. Retry before closing the app.");throw e}
    await this.retryInside();
  }
  /** Keeps a sign-in alive for a client that otherwise never needs the account service. The
   * sign-in is a 90-day rolling window renewed by any refresh, and a client in daily use against
   * its own server may go months without one. Renewing once the window is a third used costs
   * about twelve requests a year. Automatic: it defers to the shared circuit and never signs
   * anyone out on failure (only an explicit refusal does that, in rotate). */
  async keepAlive(now=Date.now()):Promise<boolean>{
    await this.ready;
    const expires=Date.parse(this.record.active?.session.refreshExpiresAt??"");
    if(!Number.isFinite(expires)||expires-now>KEEP_ALIVE_REMAINING_MS||(this.api.gate?.blockedFor()??0)>0)return false;
    try{await this.accessSession(true);return true;}catch{return false;}
  }
  async retryRevocations(){if((this.api.gate?.blockedFor()??0)>0)return;const generation=this.generation;await this.ready;return this.coordinator.runExclusive(async()=>{await this.reload(false);if(generation!==this.generation)return;this.publish();await this.retryInside()});}
  private async retryInside(){
    await Promise.all([...this.record.revocations].map(async authority=>{
      const key=JSON.stringify(authority);if(this.revoking.has(key))return;this.revoking.add(key);
      try{await this.bounded(signal=>this.api.revoke(authority,signal));this.record.revocations=this.record.revocations.filter(a=>JSON.stringify(a)!==key);await this.persist();this.publish()}
      catch{/* Keep the durable tombstone for retry. */}finally{this.revoking.delete(key)}
    }));
  }
}
