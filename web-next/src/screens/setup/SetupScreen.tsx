import {useEffect, useMemo, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button, TextField} from '../../kit/controls';
import {AuthStage} from '../../kit/accounts/AuthStage';
import {ServicesStrength} from '../../kit/accounts/ServicesAuth';
import {Icon} from '../../kit/Icon';
import {SetupFlow, type SetupPorts, type SetupSnapshot} from './setup-flow';
import {libraryKindLabel, localPasswordValid, setupLibraryKinds, setupNameValid, setupSteps, validLocalUsername} from './setup-model';
import styles from './setup.module.css';

const t = defaultI18n.t;

/** auth1 / auth1c / auth3. The mounted port bundle supplies the verified pairing,
 * the protected draft, shell admission and the guarded hosted continuation. */
export function SetupScreen({ports}: {ports: SetupPorts}) {
  const flow = useMemo(() => new SetupFlow(ports), [ports]);
  useEffect(() => {void flow.start(); return () => flow.dispose();}, [flow]);
  const state = useSyncExternalStore(flow.subscribe, flow.getSnapshot);
  const local = state.mode === 'local';
  const steps = setupSteps({serverName: state.serverName, mode: state.mode, phase: state.phase === 'library' ? 'library' : 'form', local});
  return <div className={styles.host}>
    {state.phase === 'library' ? <FirstLibrary flow={flow} state={state}/> : <AuthStage legal={false} contentClassName={state.phase === 'form' ? styles.content : styles.contentLibrary}>
      {state.phase === 'loading' && <p className={styles.lede} role="status">{t('web.setup.preparing')}</p>}
      {state.phase === 'waiting' && <p className={styles.lede} role="status">{t('web.setup.preparing')}</p>}
      {state.phase === 'error' && <>
        <h1>{t('web.setup.welcome')}</h1>
        {state.error && <p className={styles.error} role="alert"><Icon name="i-warn" size={20}/>{state.error}</p>}
        <Button variant="primary" size="lg" onClick={() => void flow.retry()}>{t('action.tryAgain')}</Button>
      </>}
      {state.phase === 'form' && <SetupForm flow={flow} state={state} local={local}/>}
    </AuthStage>}
    {state.phase === 'form' || state.phase === 'library' ? <SetupSteps steps={steps} label={t('web.setup.stepsLabel')}/> : null}
  </div>;
}

function SetupForm({flow, state, local}: {flow: SetupFlow; state: SetupSnapshot; local: boolean}) {
  const hosted = state.mode === 'hosted';
  const nameOk = setupNameValid(state.name), usernameOk = validLocalUsername(state.username), match = state.confirm === state.password;
  const canSubmit = nameOk && (hosted || usernameOk && localPasswordValid(state.password, state.confirm)) && !state.busy;
  const submit = () => {if (canSubmit) void (hosted ? flow.submitHosted() : flow.submitLocal());};
  return <form onSubmit={event => {event.preventDefault(); submit();}}>
    <h1 className={styles.one}>{t('web.setup.welcome')}</h1>
    <p className={styles.lede}>{t('web.setup.welcomeLede')}</p>
    <div className={styles.field}><TextField variant="auth" label={t('web.general.serverName')} value={state.name} maxLength={80} required
      description={t('web.setup.serverNameHelp')} onChange={event => flow.setName(event.currentTarget.value)} disabled={state.busy}/></div>
    <div className={styles.legend}>{t('web.setup.howSignIn')}</div>
    <button type="button" role="radio" aria-checked={hosted} className={hosted ? styles.choice : styles.qopt} onClick={() => flow.setMode('hosted')} disabled={state.busy}>
      <span className={styles.choiceIcon} aria-hidden="true"><Icon name="i-person" size={22}/></span>
      <span className={styles.choiceCopy}><b>{t('accounts.settings.portico')}{hosted && <span className={styles.rec}>{t('web.setup.recommended')}</span>}</b>
        <p>{t('web.setup.porticoBody')}</p></span>
      <span className={hosted ? styles.rad : styles.radOff} aria-hidden="true"/>
    </button>
    <button type="button" role="radio" aria-checked={!hosted} className={hosted ? styles.qopt : styles.choice} onClick={() => flow.setMode('local')} disabled={state.busy}>
      {!hosted && <span className={styles.choiceIcon} aria-hidden="true"><Icon name="i-server" size={22}/></span>}
      <span className={styles.qoptCopy}><b>{t('web.setup.localSignIn')}</b><br/>{t('web.setup.localSignInBody')}</span>
      <span className={!hosted ? styles.rad : styles.radOff} aria-hidden="true"/>
    </button>
    {local && <>
      <div className={[styles.legend, styles.formLegend].join(' ')}>{t('web.setup.ownerTitle')}</div>
      <p className={styles.para}>{t('web.setup.ownerLede')}</p>
      <div className={styles.field}><TextField variant="auth" label={t('web.direct.username')} value={state.username} autoComplete="username"
        autoCapitalize="off" spellCheck={false} maxLength={64} required disabled={state.busy} description={t('web.setup.usernameHelp')}
        onChange={event => flow.setUsername(event.currentTarget.value)}/></div>
      <div className={styles.field}><TextField variant="auth" type="password" label={t('web.account.password')} value={state.password}
        autoComplete="new-password" required disabled={state.busy} onChange={event => flow.setPassword(event.currentTarget.value)}/>
        <ServicesStrength password={state.password} id="setup-password-strength"/></div>
      <div className={styles.field}><TextField variant="auth" type="password" label={t('web.directJoin.confirm')} value={state.confirm}
        autoComplete="new-password" required disabled={state.busy} error={Boolean(state.confirm) && !match ? t('web.setup.mismatch') : undefined}
        onChange={event => flow.setConfirm(event.currentTarget.value)}/></div>
    </>}
    {state.error && <p className={styles.error} role="alert"><Icon name="i-warn" size={20}/>{state.error}</p>}
    <div className={[styles.btnRow, local ? styles.btnRowTight : ''].filter(Boolean).join(' ')}>
      <Button variant="primary" size="lg" type="submit" loading={state.busy} disabled={!canSubmit}>
        {hosted ? t('accounts.signIn.handoff') : t('web.setup.createOwner')}
      </Button>
    </div>
    <p className={styles.quiet}>{hosted ? t('web.setup.tabTrip') : t('web.setup.plainHttp')}</p>
  </form>;
}

function SetupSteps({steps, label}: {steps: ReturnType<typeof setupSteps>; label: string}) {
  return <div className={styles.steps} aria-label={label}>
    {steps.map(step => <div key={step.id} className={styles.step} data-state={step.state}>
      <span className={styles.stepNumber}>{step.state === 'done' ? <Icon name="i-check" size={18}/> : step.id === 'server' ? 1 : step.id === 'account' ? 2 : 3}</span>
      <span>{step.title}<small>{step.body}</small></span>
    </div>)}
  </div>;
}

function FirstLibrary({flow, state}: {flow: SetupFlow; state: SetupSnapshot}) {
  const ready = setupNameValid(state.libraryName) && state.path.trim().length > 0 && !state.busy;
  return <AuthStage legal={false} contentClassName={styles.contentLibrary}>
    <p className={styles.okline}><Icon name="i-okc" size={20}/>{t('web.setup.youOwn', {server: state.serverName})}</p>
    <h1>{t('web.setup.firstLibrary')}</h1>
    <p className={styles.lede}>{t('web.setup.firstLibraryLede')}</p>
    <div className={styles.kinds} role="group" aria-label={t('web.setup.firstLibrary')}>
      {setupLibraryKinds.map(kind => <button key={kind.id} type="button" className={styles.kind} aria-pressed={state.kind === kind.id}
        onClick={() => flow.setKind(kind.id)} disabled={state.busy}>
        <Icon name={kind.icon} size={26}/>{libraryKindLabel(kind.id)}
      </button>)}
    </div>
    <div className={styles.field}><TextField variant="auth" label={t('web.setup.folderName')} value={state.libraryName} maxLength={80}
      required disabled={state.busy} onChange={event => flow.setLibraryName(event.currentTarget.value)}/></div>
    <div className={styles.legend}>{t('web.setup.folder')}</div>
    <div className={styles.field}><TextField variant="auth" label={t('web.setup.orTypePath')} value={state.path} required disabled={state.busy}
      autoComplete="off" spellCheck={false} onChange={event => flow.setPath(event.currentTarget.value)}/></div>
    {state.error && <p className={styles.error} role="alert"><Icon name="i-warn" size={20}/>{state.error}</p>}
    <div className={styles.btnRow}>
      <Button variant="primary" size="lg" loading={state.busy} disabled={!ready} onClick={() => void flow.addLibrary()}>{t('web.setup.addLibraryScan')}</Button>
      <Button variant="glass" size="lg" disabled={state.busy} onClick={flow.skip}>{t('web.setup.skip')}</Button>
    </div>
  </AuthStage>;
}
