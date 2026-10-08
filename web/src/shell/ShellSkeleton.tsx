import React from 'react';
import {useI18n} from '../app/i18n';
import {lastRailCache} from '../app/rail-cache';
import {usePreferences} from '../app/preferences';
import {iconFor} from '@core/presentation/index.ts';
import {BrandMark, Icon, Skeleton, Wordmark, cx, useCompact} from '../ui';
import {channelEntries} from '../app/channels';
import s from './Shell.module.css';

/**
 * The shell's shape while a stored session is being restored: the rail exactly as this viewer
 * last saw it (from the rail cache), with a page-shaped skeleton where the content goes. The
 * real shell replaces it with the same geometry, so opening the app is one paint, not a card,
 * then a frame, then the rail filling in. Nothing here is interactive.
 */
export function ShellSkeleton() {
  const {t} = useI18n();
  const compact = useCompact();
  const {preferences} = usePreferences();
  const cache = lastRailCache();
  const collapsed = !preferences.railExpanded;
  const row = (icon: React.ComponentProps<typeof Icon>['name'], label: string, key: string) => (
    <span key={key} className={s.railItem} aria-hidden><Icon name={icon} size={20} /><span className={s.railLabel}>{label}</span></span>
  );
  return (
    <div className={cx(s.frame, collapsed && !compact && s.collapsed)} aria-busy aria-label={t('launch.title')}>
      {!compact ? (
        <nav className={s.rail} aria-hidden>
          <span className={s.brand}><BrandMark size={26} /><span><Wordmark height={16} /></span></span>
          <div className={s.railGroup}>
            {row('home', t('web.shell.home'), 'home')}
            {row('search', t('web.shell.search'), 'search')}
            {row('saved', t('web.shell.saved'), 'saved')}
          </div>
          <div className={s.railScroll}>
            <div className={s.railHeading}>{t('web.shell.libraries')}</div>
            <div className={s.railGroup}>
              {cache?.libraries.map(l => row(iconFor(l.kind), l.name, l.id))}
            </div>
            {cache?.sources.length ? (
              <>
                <div className={s.railHeading}>{t('channels.title')}</div>
                <div className={s.railGroup}>{channelEntries(cache.sources).map(e => row(e.icon, e.name, e.id))}</div>
              </>
            ) : null}
          </div>
          <div className={s.railFoot}>
            {row('bell', t('web.shell.notifications'), 'bell')}
            {row('settings', t('web.shell.settings'), 'settings')}
            <span className={cx(s.railItem, s.avatarItem)}><Skeleton width={24} height={24} radius={12} /><span className={s.railLabel}>{cache?.name ?? ''}</span></span>
          </div>
        </nav>
      ) : null}
      <div className={s.main}>
        {compact ? <header className={s.topBar}><Wordmark height={18} /></header> : null}
        <div className={s.content}><PageSkeleton /></div>
      </div>
    </div>
  );
}

/** A page's shape while its code or first read is on the way: a title line and two rows of cards. */
export function PageSkeleton({rows = 2}: {rows?: number}) {
  return (
    <div className={s.pageSkeleton} aria-hidden>
      <Skeleton width={220} height={32} radius={8} />
      {Array.from({length: rows}).map((_, r) => (
        <div key={r} className={s.skeletonRow}>
          <Skeleton width={160} height={20} radius={6} />
          <div className={s.skeletonCards}>{Array.from({length: 8}).map((_, i) => <Skeleton key={i} style={{aspectRatio: '2 / 3', width: '100%'}} />)}</div>
        </div>
      ))}
    </div>
  );
}
