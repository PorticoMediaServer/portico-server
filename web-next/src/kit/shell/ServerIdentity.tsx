import type {HTMLAttributes, ReactNode} from 'react';
import {createI18n, type I18n} from '@i18n';
import type {FactPart} from '@core/presentation/kit-model';
import {Icon} from '../Icon';
import styles from './identity.module.css';

export type ServerRole = 'owner' | 'administrator' | 'member';
export type ServerAudience = 'owner' | 'administrators';
export type ShellTranslate = I18n['t'];
const defaultTranslate = createI18n().t;

/** The mark carries no authority; the caller supplies the authorized area's facts. */
export function ServerMark({size = 26, className, ...props}: Omit<HTMLAttributes<HTMLSpanElement>, 'children'> & {size?: 22 | 26 | 34 | 38}) {
  const iconSize = size === 22 ? 13 : size === 26 ? 15 : size === 34 ? 18 : 20;
  return <span {...props} className={[styles.mark, className].filter(Boolean).join(' ')} data-size={size} aria-hidden="true"><Icon name="i-server" size={iconSize} /></span>;
}

/** A restriction badge, not the viewer's role. Safe on a member's denied-state page. */
export function OwnerTag({identity, variant = 'badge', className, t = defaultTranslate}: {
  identity?: ReactNode; variant?: 'badge' | 'state'; className?: string; t?: ShellTranslate;
}) {
  if (variant === 'badge') return <span className={[styles.badge, className].filter(Boolean).join(' ')}>{t('next.shell.ownerOnly')}</span>;
  return <span className={[styles.ownerState, className].filter(Boolean).join(' ')}><ServerMark />{identity}<span>{t('next.shell.ownerOnly')}</span></span>;
}

export function ServerBand({variant = 'settings', className}: {variant?: 'settings' | 'state'; className?: string}) {
  return <div className={[styles.band, className].filter(Boolean).join(' ')} data-variant={variant} aria-hidden="true"><div /></div>;
}

export type ServerHeaderProps = {
  serverName: string;
  viewerRole: Exclude<ServerRole, 'member'>;
  audience: ServerAudience;
  health: 'normal' | 'attention';
  facts?: readonly FactPart[];
  className?: string;
  t?: ShellTranslate;
};

/** Layout only: admission and the list of facts remain with the real settings model. */
export function ServerHeader({serverName, viewerRole, audience, health, facts = [], className, t = defaultTranslate}: ServerHeaderProps) {
  const visibility = viewerRole === 'administrator' ? 'next.shell.serverVisibility.administrator' : audience === 'owner' ? 'next.shell.serverVisibility.owner' : 'next.shell.serverVisibility.ownerShared';
  return <header className={[styles.header, className].filter(Boolean).join(' ')}>
    <div className={styles.serverPill}><Icon name="i-server" size={16} />{t('next.shell.server')}<span>{t(visibility)}</span></div>
    <div className={styles.nameLine}><b>{serverName}</b><div className={styles.facts}>
      <span className={styles.health} data-health={health}>{t(health === 'normal' ? 'next.shell.health.normal' : 'next.shell.health.attention')}</span>
      {facts.map(fact => <span key={fact.id} data-role={fact.role}>{fact.text}</span>)}
    </div></div>
  </header>;
}
