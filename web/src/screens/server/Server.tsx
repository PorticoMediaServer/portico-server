import React from 'react';
import s from './Server.module.css';

/**
 * The heading of a panel on a Server page (the pages themselves are the Server heading of
 * Settings: `settings-structure.ts`, `screens/settings/ServerPages.tsx`): a title when the panel
 * is not the page's only subject, one line on what it is for, and its actions.
 */
export function SectionHeader({title, lede, actions}: {title?: string; lede?: string; actions?: React.ReactNode}) {
  if (!title && !lede && !actions) return null;
  return (
    <div className={s.head}>
      <div className={s.pageTitle}>
        {title ? <h2 className={s.h2}>{title}</h2> : null}
        {lede ? <p className={s.lede}>{lede}</p> : null}
      </div>
      {actions ? <div className={s.headActions}>{actions}</div> : null}
    </div>
  );
}
