import type {I18n, MessageId} from '@i18n';

/**
 * INT gate 3: what a failed sign-in says. Every code a sign-in can meet (the server's, the
 * Portico Account service's, and client-core's own) has its own sentence here, naming the server
 * and what to do. There is no generic fallback: an unknown code is answered by its HTTP status,
 * and a failure with neither is a Portico fault, said as one. Nothing here reads `error.message`.
 *
 * `wait` says who is expected back on their own: `server` (Portico retries the server) or
 * `account` (the Portico Account service; the server is fine). `refused`: the server no longer
 * admits this Portico Account.
 */
export type SignInStep =
  | 'password' | 'portico' | 'invitation' | 'code' | 'new-password' | 'profile' | 'custody'
  | 'account' | 'account-code' | 'account-create' | 'account-recovery';
export type SignInFailure = Readonly<{silent: boolean; messageId: MessageId | ''; text: string; wait?: 'server' | 'account'; refused?: boolean}>;
export type SignInContext = {step: SignInStep; server?: string; online?: boolean};

type Id = MessageId;
type Answer = Id | ((step: SignInStep) => Id);
const E = (id: string) => id as Id;

/** Steps that ask the Portico Account service for an identity assertion first. */
const viaAccount = (step: SignInStep) => step === 'portico' || step === 'invitation' || step === 'custody';
const accountStep = (step: SignInStep) => step.startsWith('account');

/** A 400/422 from the server: the thing this step sent. */
const rejectedInput = (step: SignInStep): Id =>
  step === 'code' ? E('web.signIn.error.code')
    : step === 'new-password' ? E('web.signIn.error.newPassword')
      : step === 'profile' ? E('web.signIn.error.pinFormat')
        : viaAccount(step) ? E('web.signIn.error.porticoRejected')
          : E('web.signIn.error.details');
/** A 401 from the server. */
const notAccepted = (step: SignInStep): Id =>
  viaAccount(step) ? E('web.signIn.error.porticoRejected')
    : step === 'code' ? E('web.signIn.error.code')
      : step === 'new-password' ? E('web.signIn.error.currentPassword')
        : step === 'profile' ? E('web.signIn.error.startAgain')
          : E('web.signIn.error.credentials');

const silentCodes = new Set(['cancelled', 'canceled', 'aborted', 'abort', 'superseded']);
const serverWaits = new Set<string>(['web.signIn.error.unreachable', 'web.signIn.error.offline', 'web.signIn.error.timeout', 'web.signIn.error.busy']);
const accountWaits = new Set<string>(['web.signIn.error.accountUnavailable', 'web.signIn.error.accountUnreachable', 'web.signIn.error.accountTimeout']);

/** Codes from the server, or from client-core on the server's behalf. */
export const serverCodes: Readonly<Record<string, Answer>> = {
  unauthorized: notAccepted, unauthenticated: notAccepted, authentication_required: notAccepted,
  invalid_credentials: E('web.signIn.error.credentials'), ambiguous_credentials: E('web.signIn.error.credentials'), invalid_grant: notAccepted,
  credentials_required: E('web.signIn.error.details'),
  invalid_request: rejectedInput, validation_failed: rejectedInput, invalid_json: rejectedInput,
  two_factor_invalid: E('web.signIn.error.code'), two_factor_required: E('web.signIn.error.code'), challenge_invalid: E('web.signIn.error.code'),
  invalid_code: E('web.signIn.error.code'), mfa_invalid: E('web.signIn.error.code'), code_required: E('web.signIn.error.code'),
  expired_token: E('web.signIn.error.codeExpired'), challenge_expired: E('web.signIn.error.codeExpired'), operation_expired: E('web.signIn.error.codeExpired'),
  password_change_required: E('web.signIn.error.startAgain'), session_migrated: E('web.signIn.error.startAgain'), device_not_found: E('web.signIn.error.startAgain'),
  session_expired: E('web.signIn.error.startAgain'), invalid_token: E('web.signIn.error.startAgain'),
  current_password_incorrect: E('web.signIn.error.currentPassword'),
  rate_limited: E('web.signIn.error.tooMany'), too_many_requests: E('web.signIn.error.tooMany'), identity_credential_attempts: E('web.signIn.error.tooMany'),
  identity_account_attempt_budget: E('web.signIn.error.tooMany'), profile_pin_locked: E('web.signIn.error.tooMany'),
  authentication_busy: E('web.signIn.error.busy'), server_busy: E('web.signIn.error.busy'), busy: E('web.signIn.error.busy'),
  device_approval_pending: E('device.approval.body'),
  device_denied: E('web.signIn.error.deviceDenied'),
  access_refused: E('web.portico.noAccess'),
  clock_skew: E('web.portico.clockSkew'),
  challenge_not_proven: E('web.portico.notProven'),
  server_mismatch: E('web.signIn.error.identity'), identity_mismatch: E('web.signIn.error.identity'), session_identity_mismatch: E('web.signIn.error.identity'),
  wrong_server: E('web.signIn.error.identity'), invalid_signature: E('web.signIn.error.identity'), missing_identity: E('web.signIn.error.identity'),
  invalid_identity: E('web.signIn.error.identity'), missing_server_key: E('web.signIn.error.identity'), route_stale: E('web.signIn.error.identity'),
  invalid_routes: E('web.signIn.error.identity'),
  invalid_response: E('web.signIn.error.unreadable'), unreadable_response: E('web.signIn.error.unreadable'), response_too_large: E('web.signIn.error.unreadable'),
  invalid_scope: E('web.signIn.error.unreadable'), invalid_saved_connection: E('web.signIn.error.startAgain'),
  not_portico_server: E('web.signIn.error.notPortico'),
  invalid_address: E('web.signIn.error.address'),
  network: E('web.signIn.error.unreachable'), request_failed: E('web.signIn.error.unreachable'), route_unavailable: E('web.signIn.error.unreachable'),
  server_unavailable: E('web.signIn.error.unreachable'), remote_unavailable: E('web.signIn.error.unreachable'), console_unavailable: E('web.signIn.error.unreachable'),
  timeout: E('web.signIn.error.timeout'), remote_stalled: E('web.signIn.error.timeout'),
  setup_required: E('web.signIn.error.notSetUp'),
  unsupported_version: E('web.signIn.error.update'), client_too_old: E('web.signIn.error.update'), client_update_required: E('web.signIn.error.update'),
  invitation_expired: E('web.signIn.error.invitationExpired'), invitation_incomplete: E('web.signIn.error.invitationIncomplete'),
  administration_conflict: E('web.signIn.error.invitationUsed'),
  registration_closed: E('web.signIn.error.closed'), registration_invitation_required: E('web.signIn.error.closed'),
  username_taken: E('web.signIn.error.usernameTaken'),
  profile_pin_invalid: E('signIn.error.pinInvalid'), invalid_pin: E('signIn.error.pinInvalid'), profile_pin: E('signIn.error.pinInvalid'),
  profile_pin_required: E('web.signIn.error.pinRequired'), pin_format: E('web.signIn.error.pinFormat'),
  profile_changed: E('web.signIn.error.profilesChanged'), profile_selection_required: E('web.signIn.error.profilesChanged'),
  primary_profile: E('web.signIn.error.profilesChanged'), restrictions_changed: E('web.signIn.error.profilesChanged'),
  remote_sign_in_not_allowed: E('web.signIn.error.remoteNotAllowed'),
  feature_restricted: E('web.signIn.error.forbidden'), owner_required: E('web.signIn.error.forbidden'), forbidden: E('web.signIn.error.forbidden'),
  permission_denied: E('web.signIn.error.forbidden'), access_limit: E('web.signIn.error.forbidden'), access_revoked: E('web.portico.noAccess'),
  origin_not_allowed: E('web.signIn.error.origin'),
  account_signed_out: E('web.portico.signInFirst'), account_signout_pending: E('web.signIn.error.signOutPending'),
  account_session_changed: E('web.signIn.error.accountChanged'),
  browser_storage: E('web.signIn.error.storage'),
  storage_full: E('web.signIn.error.serverFull'), insufficient_storage: E('web.signIn.error.serverFull'),
  persistence_error: E('web.signIn.error.serverFault'), internal_error: E('web.signIn.error.serverFault'), unexpected: E('web.signIn.error.serverFault'),
  conflict: E('web.signIn.error.changed'), refresh_required: E('web.signIn.error.changed'), stale: E('web.signIn.error.changed'),
};

/** Codes from the Portico Account service (Hosted), on its sign-in forms or asked for a server's identity assertion. */
export const accountCodes: Readonly<Record<string, Answer>> = {
  invalid_credentials: E('web.signIn.error.accountCredentials'), ambiguous_credentials: E('web.signIn.error.accountCredentials'),
  credentials_required: E('web.signIn.error.accountDetails'),
  invalid_grant: E('web.signIn.error.accountEnded'), refresh_reuse_detected: E('web.signIn.error.accountEnded'), unauthorized: E('web.signIn.error.accountEnded'),
  family_changed: E('web.signIn.error.accountEnded'), session_mode_denied: E('web.signIn.error.accountEnded'), account_signed_out: E('web.portico.signInFirst'),
  account_session_changed: E('web.signIn.error.accountChanged'), account_signout_pending: E('web.signIn.error.signOutPending'),
  challenge_invalid: E('web.signIn.error.code'), code_required: E('web.signIn.error.code'),
  expired_token: E('signIn.error.expired'), challenge_expired: E('signIn.error.expired'), continuation_required: E('signIn.error.expired'),
  continuation_denied: E('signIn.error.expired'), invalid_link: E('signIn.error.expired'),
  rate_limited: E('web.signIn.error.accountTooMany'), too_many_requests: E('web.signIn.error.accountTooMany'),
  capacity_limited: E('web.signIn.error.accountUnavailable'), unavailable: E('web.signIn.error.accountUnavailable'),
  account_service_recovering: E('web.signIn.error.accountUnavailable'), server_list_unavailable: E('web.signIn.error.accountUnavailable'),
  mail_unavailable: E('web.signIn.error.accountMail'), push_unavailable: E('web.signIn.error.accountUnavailable'),
  membership_busy: E('web.signIn.error.accountBusy'),
  permission_denied: step => (viaAccount(step) ? E('web.signIn.error.accountNotRegistered') : E('web.signIn.error.accountEnded')),
  not_found: step => (viaAccount(step) ? E('web.signIn.error.accountNotRegistered') : E('web.signIn.error.accountBug')),
  invalid_input: step => (step === 'account-create' ? E('web.signIn.error.accountCreateDetails') : step === 'account-recovery' ? E('web.signIn.error.accountRecoveryDetails') : viaAccount(step) ? E('web.signIn.error.porticoRejected') : E('web.signIn.error.accountDetails')),
  invalid_request: step => (step === 'account-create' ? E('web.signIn.error.accountCreateDetails') : step === 'account-recovery' ? E('web.signIn.error.accountRecoveryDetails') : viaAccount(step) ? E('web.signIn.error.porticoRejected') : E('web.signIn.error.accountDetails')),
  invalid_json: E('web.signIn.error.accountBug'), invalid_content_type: E('web.signIn.error.accountBug'), content_type: E('web.signIn.error.accountBug'),
  conflict: step => (step === 'account-create' ? E('web.signIn.error.accountTaken') : E('web.signIn.error.accountChanged')),
  username_taken: E('web.signIn.error.accountTaken'),
  provider_unavailable: E('web.signIn.error.accountProvider'), native_required: E('web.signIn.error.accountProvider'),
  csrf_required: E('web.signIn.error.reload'), origin_denied: E('web.signIn.error.reload'),
  invalid_response: E('web.signIn.error.accountBug'),
  network: E('web.signIn.error.accountUnreachable'), request_failed: E('web.signIn.error.accountUnreachable'),
  timeout: E('web.signIn.error.accountTimeout'),
  browser_storage: E('web.signIn.error.storage'),
};

function serverByStatus(status: number, step: SignInStep): Id {
  if (status === 401) return notAccepted(step);
  if (status === 403) return E('web.signIn.error.forbidden');
  if (status === 404 || status === 405 || status === 410) return step === 'invitation' ? E('web.signIn.error.invitationUsed') : viaAccount(step) ? E('web.portico.serverUpdate') : E('web.signIn.error.serverOld');
  if (status === 409 || status === 412) return E('web.signIn.error.changed');
  if (status === 426) return E('web.signIn.error.update');
  if (status === 429) return E('web.signIn.error.tooMany');
  if (status === 408 || status === 504) return E('web.signIn.error.timeout');
  if (status === 502 || status === 503) return E('web.signIn.error.busy');
  if (status === 507) return E('web.signIn.error.serverFull');
  if (status >= 500) return E('web.signIn.error.serverFault');
  return rejectedInput(step);
}

function accountByStatus(status: number, step: SignInStep): Id {
  if (status === 401) return E('web.signIn.error.accountEnded');
  if (status === 403 || status === 404 || status === 410) return viaAccount(step) ? E('web.signIn.error.accountNotRegistered') : E('web.signIn.error.reload');
  if (status === 409 || status === 412) return E('web.signIn.error.accountChanged');
  if (status === 429) return E('web.signIn.error.accountTooMany');
  if (status === 408 || status === 504) return E('web.signIn.error.accountTimeout');
  if (status >= 500) return E('web.signIn.error.accountUnavailable');
  return (accountCodes.invalid_input as (s: SignInStep) => Id)(step);
}

/** Whether a failure came from the Portico Account service (tagged by the caller's Hosted transport). */
export const fromAccount = (e: unknown): boolean => (e as {service?: unknown} | null)?.service === 'portico-account';
/** Marks a failure as the Portico Account service's, so it is never said as the server's. */
export function accountFailure<T>(e: T): T {
  if (e && typeof e === 'object') { try { Object.defineProperty(e, 'service', {value: 'portico-account', configurable: true}); } catch { /* frozen: left as is */ } }
  return e;
}

/** A cancelled or superseded sign-in (the person moved on): nothing to say. */
export const superseded = () => Object.assign(new Error('The selected server changed.'), {code: 'superseded'});

export function signInFailure(error: unknown, context: SignInContext, i18n: Pick<I18n, 't'>): SignInFailure {
  const e = (error && typeof error === 'object' ? error : {}) as {code?: unknown; status?: unknown; name?: unknown; retryAfterSeconds?: unknown};
  const code = typeof error === 'string' ? error : typeof e.code === 'string' ? e.code : '';
  const status = typeof e.status === 'number' ? e.status : undefined;
  const name = typeof e.name === 'string' ? e.name : '';
  const step = context.step;
  if (silentCodes.has(code) || name === 'AbortError') return Object.freeze({silent: true, messageId: '', text: ''});
  const account = fromAccount(error) || (accountStep(step) && !code.startsWith('device_'));
  const table = account ? accountCodes : serverCodes;
  let id: Id;
  const known = code ? table[code] : undefined;
  if (known) id = typeof known === 'function' ? known(step) : known;
  else if (status !== undefined) id = account ? accountByStatus(status, step) : serverByStatus(status, step);
  else if (name === 'TimeoutError') id = account ? E('web.signIn.error.accountTimeout') : E('web.signIn.error.timeout');
  else if (error instanceof TypeError) id = account ? E('web.signIn.error.accountUnreachable') : E('web.signIn.error.unreachable');
  else id = account ? E('web.signIn.error.accountBug') : E('web.signIn.error.bug');
  if (context.online === false && (id === 'web.signIn.error.unreachable' || id === 'web.signIn.error.accountUnreachable')) id = E('web.signIn.error.offline');
  const seconds = typeof e.retryAfterSeconds === 'number' && Number.isFinite(e.retryAfterSeconds) && e.retryAfterSeconds > 0 ? Math.ceil(e.retryAfterSeconds) : undefined;
  if (seconds && (id === 'web.signIn.error.tooMany' || id === 'web.signIn.error.accountTooMany')) id = E('web.signIn.error.tooManySeconds');
  const server = context.server || i18n.t(E('web.signIn.error.yourServer'));
  const raw = i18n.t(id, {server, seconds: seconds ?? 0});
  // "your server" may open a sentence; a server's own name is left as written.
  const text = context.server ? raw : raw.charAt(0).toUpperCase() + raw.slice(1);
  const wait = serverWaits.has(id) ? 'server' as const : accountWaits.has(id) ? 'account' as const : undefined;
  return Object.freeze({silent: false, messageId: id, text, ...(wait ? {wait} : {}), ...(id === 'web.portico.noAccess' ? {refused: true} : {})});
}
