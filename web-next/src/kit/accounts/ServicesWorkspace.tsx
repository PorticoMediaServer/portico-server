import type {ReactNode} from 'react';
import {createI18n, deviceRegion, resolveRegion} from '@i18n';
import {Icon, type IconName} from '../Icon';
import {ProfileAvatarFace} from '../shell/AccountSurfaces';
import {SettingsRow} from '../shell/SettingsSurfaces';
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
export function ServicesHeader({avatar, unread = false}: {avatar: ReactNode; unread?: boolean}) {
  return <header className={styles.header}>
    <a className={styles.brand} href="/" aria-label={t('next.platform.appName')}><img src={wordmark} alt="" /></a>
    <a className={styles.home} href="/">{t('web.shell.home')}</a>
    <div className={styles.tools}><a className={styles.bell} href="/account?tab=notifications" aria-label={t('services.workspace.notifications')}><Icon name="i-bell" size={22} />{unread && <span className={styles.unread} />}</a>{avatar}</div>
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
