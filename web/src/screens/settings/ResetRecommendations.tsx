import {useRef, useState} from 'react';
import {resetRecommendations} from '@core/recommendation-feedback.ts';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {clearNotInterested, recommendationViewer} from '../../app/not-interested';
import {currentI18n} from '../../app/i18n';
import {Button, ConfirmDialog, SettingsRow} from '../../ui';

/**
 * Settings › Privacy: "Reset recommendations" (Recommendations P5). Behind a confirmation that
 * says what stays; the server forgets the profile's learned taste and every Not interested mark.
 * One operation ID per confirmation, so a retry after a lost answer can't reset twice.
 */
export function ResetRecommendationsRow() {
  const {api, session} = useSession();
  const viewer = recommendationViewer(useViewerScope());
  const t = currentI18n().t;
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [done, setDone] = useState(false);
  const operation = useRef<string | undefined>(undefined);
  const ask = () => { operation.current = crypto.randomUUID(); setError(undefined); setDone(false); setOpen(true); };
  const confirm = async () => {
    setBusy(true);
    setError(undefined);
    try {
      await resetRecommendations(api, {serverId: session?.viewer.serverId ?? '', operationId: operation.current ?? crypto.randomUUID()});
      clearNotInterested(viewer);
      setOpen(false);
      setDone(true);
    } catch {
      setError(t('settings.resetRecommendations.failed'));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <SettingsRow label={t('settings.resetRecommendations')} help={done ? t('settings.resetRecommendations.done') : t('settings.resetRecommendations.help')} control={<Button size="sm" variant="secondary" label={t('settings.resetRecommendations.confirm')} onClick={ask} />} />
      <ConfirmDialog open={open} onOpenChange={o => { if (!busy) setOpen(o); }} title={t('settings.resetRecommendations.confirmTitle')} body={t('settings.resetRecommendations.confirmBody')} confirmLabel={t('settings.resetRecommendations.confirm')} cancelLabel={t('action.cancel')} onConfirm={confirm} busy={busy} error={error} />
    </>
  );
}
