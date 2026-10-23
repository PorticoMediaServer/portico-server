import type {ReactNode} from 'react';
import {createI18n, deviceRegion, resolveRegion} from '@i18n';
import {Icon, type IconName} from '../Icon';
import {ProfileAvatarFace} from '../shell/AccountSurfaces';
import {SettingsRow} from '../shell/SettingsSurfaces';
import {Button, TextField} from '../controls';
import wordmark from '../../shell/wordmark.svg';
import styles from './services-workspace.module.css';

const t = createI18n(resolveRegion(undefined, deviceRegion())).t;
export type ServicesSection = 'devices' | 'security' | 'servers' | 'notifications';
const sections: readonly {id: ServicesSection; icon: IconName; label: string}[] = [
  {id: 'devices', icon: 'i-laptop', label: t('services.workspace.devices')},
  {id: 'security', icon: 'i-shield', label: t('services.workspace.security')},
  {id: 'servers', icon: 'i-server', label: t('services.workspace.servers')},
  {id: 'notifications', icon: 'i-bell', label: t('services.workspace.notifications')},
];

/** No server/library controls exist in the Services header. */
export function ServicesHeader({avatar, unread = false, notifications = true}: {avatar: ReactNode; unread?: boolean; notifications?: boolean}) {
  return <header className={styles.header}>
    <a className={styles.brand} href="/" aria-label={t('next.platform.appName')}><img src={wordmark} alt="" /></a>
    <a className={styles.home} href="/">{t('web.shell.home')}</a>
    <div className={styles.tools}>{notifications && <a className={styles.bell} href="/account?tab=notifications" aria-label={t('services.workspace.notifications')}><Icon name="i-bell" size={22} />{unread && <span className={styles.unread} />}</a>}{avatar}</div>
  </header>;
}

export function ServicesWorkspace({header, account, section, onSection, onSignOut, description, actions, children}: {
  header: ReactNode; account: {name: string; email?: string; initial: string}; section: ServicesSection;
  onSection: (section: ServicesSection) => void; onSignOut: () => void; description?: ReactNode; actions?: ReactNode; children: ReactNode;
}) {
  const selected = sections.find(item => item.id === section)!;
  return <div className={styles.workspace}>{header}<div className={styles.glow} aria-hidden="true" />
    <div className={styles.workspaceGrid}><aside className={styles.aside}>
      <h1>{t('services.workspace.title')}</h1><div className={styles.who}><ProfileAvatarFace initial={account.initial} size={48} /><span><b>{account.name}</b>{account.email && <small>{account.email}</small>}</span></div>
      <nav aria-label={t('services.workspace.sections')}>{sections.map(item => <button type="button" key={item.id} aria-current={item.id === section ? 'page' : undefined} onClick={() => onSection(item.id)}><Icon name={item.icon} size={19} />{item.label}</button>)}<div className={styles.navSpacer} /><button type="button" onClick={onSignOut}><Icon name="i-signout" size={19} />{t('action.signOut')}</button></nav>
    </aside><main className={styles.pane}>
      <span className={styles.pill}><Icon name="i-person" size={16} />{t('services.workspace.title')}<span>{t('services.workspace.subtitle')}</span></span>
      <div className={styles.heading}><h2>{selected.label}</h2>{actions && <div>{actions}</div>}</div>{description && <p className={styles.lead}>{description}</p>}
      <nav className={styles.phoneTabs} aria-label={t('services.workspace.sections')}>{sections.map(item => <button key={item.id} type="button" aria-current={item.id === section ? 'page' : undefined} onClick={() => onSection(item.id)}>{item.label}</button>)}</nav>
      {children}
    </main></div>
  </div>;
}

export function ServicesGroup({title, description, actions, danger = false, children}: {title?: ReactNode; description?: ReactNode; actions?: ReactNode; danger?: boolean; children: ReactNode}) {
  return <section className={styles.group}>{(title || actions) && <div className={styles.groupHeading}>{title && <h3>{title}</h3>}{actions && <div>{actions}</div>}</div>}{description && <p className={styles.groupDescription}>{description}</p>}<div className={styles.groupBox} data-danger={danger || undefined}>{children}</div></section>;
}
export function ServicesWorkspaceRow({title, help, children, icon, badge}: {title: ReactNode; help?: ReactNode; children?: ReactNode; icon?: IconName; badge?: ReactNode}) {
  if (!icon) return <SettingsRow label={<>{title}{badge}</>} help={help}>{children}</SettingsRow>;
  return <div className={styles.iconRow}><span className={styles.icon}><Icon name={icon} size={24} /></span><div><b>{title}{badge}</b>{help && <p>{help}</p>}</div><div>{children}</div></div>;
}
export function ServicesBadge({children, tone = 'default'}: {children: ReactNode; tone?: 'default' | 'accent' | 'healthy'}) { return <span className={styles.badge} data-tone={tone}>{children}</span>; }
export function ServicesEmpty({title, description, actions}: {title: ReactNode; description: ReactNode; actions?: ReactNode}) { return <div className={styles.empty}><b>{title}</b><p>{description}</p>{actions}</div>; }
export function ServicesFeedRow({unread, icon, tone = 'blue', title, body, when, actions}: {unread: boolean; icon: IconName; tone?: 'blue' | 'green' | 'amber'; title: ReactNode; body: ReactNode; when: ReactNode; actions?: ReactNode}) {
  return <article className={styles.feedRow} data-unread={unread || undefined}><span className={styles.icon} data-tone={tone}><Icon name={icon} size={24} /></span><div><b>{title}</b><p>{body}</p>{actions && <div className={styles.feedActions}>{actions}</div>}</div><time>{when}</time></article>;
}

const downloads = [
  {id: 'windows', title: 'services.start.windows', detail: 'services.start.windowsDetail'},
  {id: 'macos', title: 'services.start.macos', detail: 'services.start.macosDetail'},
  {id: 'linux', title: 'services.start.linux', detail: 'services.start.linuxDetail'},
  {id: 'docker', title: 'services.start.docker', detail: 'services.start.dockerDetail'},
] as const;
const apps = [{icon: 'i-apple', title: 'services.start.iphone'}, {icon: 'i-phone', title: 'services.start.android'}, {icon: 'i-tv', title: 'services.platform.tvos'}, {icon: 'i-tvlib', title: 'services.start.tv'}] as const;
const steps = [{title: 'services.start.install', body: 'services.start.installBody'}, {title: 'services.start.name', body: 'services.start.nameBody'}, {title: 'services.start.library', body: 'services.start.libraryBody'}] as const;

/** Exact connect.html69f2d45 symbols, kept inside this owned kit module. */
function DownloadIcon({kind}: {kind: (typeof downloads)[number]['id']}) {
  if (kind === 'macos') return <Icon name="i-apple" size={26} />;
  return <svg viewBox="0 0 24 24" width={26} height={26} aria-hidden="true" focusable="false">
    {kind === 'windows' ? <g fill="currentColor"><rect x="3.5" y="3.5" width="7.8" height="7.8" rx="1.2" /><rect x="12.7" y="3.5" width="7.8" height="7.8" rx="1.2" /><rect x="3.5" y="12.7" width="7.8" height="7.8" rx="1.2" /><rect x="12.7" y="12.7" width="7.8" height="7.8" rx="1.2" /></g>
      : kind === 'linux' ? <g fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="m7 9 3 3-3 3M12.5 15H17" /></g>
      : <g fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round"><path d="M12 3 20 7.5v9L12 21l-8-4.5v-9z" /><path d="M4.4 7.7 12 12l7.6-4.3M12 12v8.6" /></g>}
  </svg>;
}
function StartGlyph({kind, size = kind === 'paste' ? 20 : 22}: {kind: 'link' | 'paste' | 'server'; size?: number}) {
  return <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true" focusable="false"><g fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin={kind === 'paste' ? 'round' : undefined}>
    {kind === 'link' ? <path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1" /> : kind === 'server' ? <><rect x="3" y="4" width="18" height="7" rx="2" /><rect x="3" y="13" width="18" height="7" rx="2" /><path d="M7 7.5h.01M7 16.5h.01" /></> : <><rect x="5" y="4.5" width="14" height="16" rx="2" /><path d="M9 4.5V3.5h6v1M8.5 10h7M8.5 14h5" /></>}
  </g></svg>;
}

/** connect26/31: controlled content only; invitation preview/session belongs to Apps. */
export function ServicesStart({header, invitation, onInvitationChange, onJoin, onPaste, onDownload, onApps, onCopyDownload, onShareDownload, error, children}: {
  header: ReactNode; invitation: string; onInvitationChange: (value: string) => void; onJoin: () => void; onPaste: () => void;
  onDownload: () => void; onApps: () => void; onCopyDownload: () => void; onShareDownload: () => void; error?: ReactNode; children?: ReactNode;
}) {
  return <div className={styles.start}>{header}<div className={styles.startGlow} aria-hidden="true" />
    <main className={styles.startMain}><div className={styles.startIntro}><div className={styles.startHeading}><p>{t('services.start.ready')}</p><h1>{t('services.start.welcome')}</h1><p className={styles.startLead}>{t('services.start.body')}<span className={styles.startDesktop}> {t('services.start.free')}</span></p></div>
      <div className={styles.startCards}><section className={styles.startJoin} id="join">
        <div className={styles.startCardHeading}><span className={styles.startIcon}><StartGlyph kind="link" /></span><h2>{t('services.start.join')}</h2></div><p>{t('services.start.joinBody')}</p>
        <form className={styles.startJoinForm} onSubmit={event => { event.preventDefault(); onJoin(); }}>
          <TextField variant="auth" aria-label={t('services.start.invitation')} placeholder={t('services.start.invitation')} value={invitation} onChange={event => onInvitationChange(event.currentTarget.value)} autoCapitalize="off" autoComplete="off" spellCheck={false}
            trailing={<button type="button" className={styles.paste} aria-label={t('services.start.paste')} onClick={onPaste}><StartGlyph kind="paste" /></button>} />
          <Button variant="primary" type="submit">{t('services.start.joinAction')}</Button>
        </form>{error}
        <p className={styles.startQuiet}><span className={styles.startDesktop}>{t('services.start.qr')}</span><span className={styles.startPhone}>{t('services.start.qrPhone')}</span></p>
        <div className={styles.startWhere}><b>{t('services.start.where')}</b><p>{t('services.start.whereBody')}</p></div>
      </section><section className={styles.startDownloads} id="setup">
        <span className={styles.startIcon}><StartGlyph kind="server" /></span><h2>{t('services.start.setup')}</h2><p>{t('services.start.setupBody')}</p>
        <div className={styles.downloads}>{downloads.map(item => <div key={item.id}><DownloadIcon kind={item.id} /><b>{t(item.title)}</b><small>{t(item.detail)}</small><Button size="small" onClick={onDownload}>{t('action.download')}</Button></div>)}</div>
        <div className={styles.startSteps}>{steps.map((item, index) => <div key={item.title}><b><i>{index + 1}</i>{t(item.title)}</b><p>{t(item.body)}</p></div>)}</div>
      </section></div>
      <div className={styles.mobileSetup}><h2>{t('services.start.setup')}</h2><button type="button" className={styles.mobileSetupLink} onClick={() => document.getElementById('setup-details')?.scrollIntoView({block: 'start'})}><span className={styles.mobileRowIcon}><StartGlyph kind="server" size={18} /></span><span>{t('services.start.serverFree')}<small>{t('services.start.notPhone')}</small></span><Icon name="i-right" size={18} /></button></div></div>
      <section className={styles.mobileSetupDetails} id="setup-details"><h2>{t('services.start.setup')}</h2><p>{t('services.start.phoneSetupBody')}</p>
        <div className={styles.mobileSetupActions}><Button size="small" icon="i-copy" onClick={onCopyDownload}>{t('services.start.copyLink')}</Button><Button size="small" icon="i-share" onClick={onShareDownload}>{t('services.start.shareLink')}</Button></div>
        <h3>{t('services.start.whatNext')}</h3><div className={styles.mobileSteps}>{steps.map((item,index) => <div key={item.title}><i>{index + 1}</i><span>{t(item.title)}<small>{t(item.body)}</small></span></div>)}</div>
        <h3>{t('services.start.apps')}</h3><div className={styles.mobileAppLinks}><button type="button" onClick={onApps}><span className={styles.mobileRowIcon}><Icon name="i-apple" size={18} /></span><span>{t('services.start.porticoIPhone')}</span><Icon name="i-right" size={18} /></button><button type="button" onClick={onApps}><span className={styles.mobileRowIcon}><Icon name="i-tv" size={18} /></span><span>{t('services.start.porticoTV')}<small>{t('services.start.allTVs')}</small></span><Icon name="i-right" size={18} /></button></div>
      </section>
      <div className={styles.startApps} id="apps"><span><b>{t('services.start.apps')}</b><small>{t('services.start.appsBody')}</small></span>{apps.map(item => <Button key={item.title} icon={item.icon} iconSize={20} size="small" onClick={onApps}>{t(item.title)}</Button>)}</div>
      <footer className={styles.startLegal}><a href="https://getportico.tv/terms/">{t('services.signup.terms')}</a><a href="https://getportico.tv/privacy/">{t('services.start.privacy')}</a></footer>
    </main>{children}
  </div>;
}
