import React from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {StateView} from '../../ui';

/**
 * ONB-07: with no libraries, a screen says what's missing and who can fix it. The owner gets
 * the way to the fix (Server › Libraries); a member is told the owner shares libraries.
 */
export function NoLibraries({where}: {where: 'home' | 'search'}) {
  const {owner} = useSession();
  const i18n = useI18n();
  const navigate = useNavigate();
  if (owner) return <StateView icon="library" title={i18n.t(where === 'search' ? 'web.empty.searchOwnerTitle' : 'web.empty.ownerTitle')} body={i18n.t('web.empty.ownerBody')} action={{label: i18n.t('web.empty.addLibrary'), onClick: () => void navigate({to: '/settings/$section', params: {section: 'server-libraries'}, search: {}})}} />;
  return <StateView icon="library" title={i18n.t('web.empty.memberTitle')} body={i18n.t('web.empty.memberBody')} />;
}

/** An empty library: the owner can open its settings to scan it; a member waits. */
export function useEmptyLibraryCopy(): {body: string; action?: {label: string; onClick: () => void}} {
  const {owner} = useSession();
  const i18n = useI18n();
  const navigate = useNavigate();
  return owner
    ? {body: i18n.t('web.empty.libraryOwnerBody'), action: {label: i18n.t('web.empty.openLibraries'), onClick: () => void navigate({to: '/settings/$section', params: {section: 'server-libraries'}, search: {}})}}
    : {body: i18n.t('web.empty.libraryMemberBody')};
}
