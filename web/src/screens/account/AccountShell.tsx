import React from 'react';
import {Link} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {BrandMark, Inset, ListRow, Page, PageHeader, Segmented, Wordmark, useNarrow} from '../../ui';

const t = defaultI18n.t;

export type AccountSection = {id: string; label: string; badge?: string};

/**
 * The light account shell (WEB-AUTH-02): the Portico brand plus a left
 * section nav when no server session is open. On narrow viewports the nav
 * becomes the section switcher above the content.
 */
export function AccountShell({username, sections, section, onSection, children}: {
  username: string;
  sections: readonly AccountSection[];
  section: string;
  onSection: (id: string) => void;
  children: React.ReactNode;
}) {
  const narrow = useNarrow();
  const current = sections.find(s => s.id === section) ?? sections[0]!;
  return (
    <Page>
      <div style={{display: 'flex', flexDirection: narrow ? 'column' : 'row', gap: narrow ? 16 : 24, alignItems: 'flex-start', width: '100%'}}>
        {!narrow ? (
          <nav aria-label={t('web.hostedAccount.section')} style={{width: 240, flexShrink: 0, position: 'sticky', top: 16, display: 'flex', flexDirection: 'column', gap: 16}}>
            <Link to="/" aria-label={t('web.hostedAccount.back')} style={{display: 'flex', alignItems: 'center', gap: 8, padding: '20px 12px 0', textDecoration: 'none'}}>
              <BrandMark size={26} />
              <Wordmark height={16} />
            </Link>
            <div style={{display: 'flex', flexDirection: 'column', gap: 4}}>
              {sections.map(s => (
                <ListRow key={s.id} title={s.label} meta={s.badge} selected={s.id === section} onClick={() => onSection(s.id)} />
              ))}
            </div>
          </nav>
        ) : null}
        <div style={{flex: 1, minWidth: 0, width: narrow ? '100%' : undefined, display: 'flex', flexDirection: 'column', gap: 16}}>
          {narrow ? (
            <div style={{display: 'flex', alignItems: 'center', gap: 8, padding: '20px 0 0'}}>
              <Link to="/" aria-label={t('web.hostedAccount.back')} style={{display: 'flex', alignItems: 'center', gap: 8, textDecoration: 'none'}}>
                <BrandMark size={26} />
                <Wordmark height={16} />
              </Link>
            </div>
          ) : null}
          <PageHeader title={current.label} eyebrow={username} />
          {narrow ? (
            <div style={{padding: '0 var(--page-gutter)'}}>
              <Segmented label={t('web.hostedAccount.section')} options={sections.map(s => ({id: s.id, label: s.badge ? `${s.label} · ${s.badge}` : s.label}))} value={section} onChange={onSection} />
            </div>
          ) : null}
          <Inset>{children}</Inset>
        </div>
      </div>
    </Page>
  );
}
