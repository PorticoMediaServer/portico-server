import {useState} from 'react';
import {isStatePermissionsAlert} from '@core/administration.ts';
import {fixStatePermissions} from '@core/backups.ts';
import type {Alert} from '@core/console.ts';
import {problem, useConsole} from '../../admin/console';
import {createOperationIds} from './operation-ids';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Notice} from '../../ui';

/** The §6.2 state-permissions warning: shown only while the server reports the item in the
 * health/alerts contract. The console explains why open permissions matter, offers the
 * owner-only "Fix permissions" repair, or dismisses the warning for this session.
 * (The contract is unpublished as of this lane; the fix route is marked TODO in client-core
 * and the open question is listed in the M33 results.) */
export function StatePermissionsCard({alert}: {alert: Alert}) {
  const {api, system, session} = useSession();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const client = useConsole();
  const {t} = useI18n();
  // One ID per logical write; a retry of the same fix reuses it (M19).
  const [opIds] = useState(createOperationIds);
  const [fixing, setFixing] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [dismissed, setDismissed] = useState(false);
  if (dismissed || !isStatePermissionsAlert(alert)) return null;
  const fix = () => {
    if (fixing) return;
    setFixing(true);
    setError('');
    setNotice('');
    const key = `fix-permissions-${alert.id}`;
    fixStatePermissions(api, serverId, opIds.forPayload(key))
      .then(() => {
        opIds.release();
        setNotice(t('web.maintenance.permissionsFixed'));
      })
      .catch(e => {
        opIds.release();
        setError(problem(e, 'action'));
      })
      .finally(() => setFixing(false));
  };
  const dismiss = () => {
    setDismissed(true);
    void client.acknowledge(alert).catch(() => {});
  };
  return (
    <Notice tone="warning" title={t('web.maintenance.permissionsTitle')} action={{label: t('web.maintenance.fixPermissions'), onClick: fix, loading: fixing}} secondaryAction={{label: t('action.dismiss'), onClick: dismiss}}>
      {error || notice || t('web.maintenance.permissionsBody')}
    </Notice>
  );
}
