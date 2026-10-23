import {useEffect, useMemo, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {PersonalSavedService, type SavedDefinition} from '@core/personal-saved.ts';
import {randomId} from '@core/random-id.ts';
import {Button, TextField} from '../../kit/controls';
import {Dialog} from '../../kit/overlays';
import type {ScreenContext} from '../detail/context';
import {screenLifetime} from '../detail/context';

/** A fresh dialog owner never changes Saved's or Home Customize's directory. */
export function SaveView({context, definition, onClose}: {context: ScreenContext; definition: SavedDefinition; onClose: () => void}) {
  const owner = useMemo(() => {
    const life = screenLifetime(context);
    const service = new PersonalSavedService(life.api, context.scope, async () => randomId());
    return {service, life};
  }, [context]);
  useEffect(() => () => {owner.life.dispose();owner.service.dispose();}, [owner]);
  const [name, setName] = useState(''), [busy, setBusy] = useState(false);
  const pending = useRef(false), input = useRef<HTMLInputElement>(null), t = defaultI18n.t;
  const save = () => {
    if (pending.current || !name.trim() || !owner.life.current()) return;
    pending.current = true;setBusy(true);
    void (async () => {
      await owner.service.select({view: 'views'});owner.life.assert();
      if (owner.service.getSnapshot().error) throw owner.service.getSnapshot().error;
      if (!await owner.service.mutate({action: 'create', kind: 'view', name: name.trim(), definition})) throw owner.service.getSnapshot().mutationError;
      owner.life.assert();onClose();
    })().catch(error => {if (owner.life.current()) context.effects.reportError(error);})
      .finally(() => {pending.current = false;if (owner.life.current()) setBusy(false);});
  };
  return <Dialog open onOpenChange={open => {if (!open && !busy) onClose();}} title={t('lib.saveViewTitle')} description={t('lib.saveViewLede')} width={420} initialFocusRef={input}
    footer={<><Button size="small" disabled={busy} onClick={onClose}>{t('action.cancel')}</Button><Button variant="primary" size="small" loading={busy} disabled={!name.trim()} onClick={save}>{t('lib.saveView')}</Button></>}>
    <TextField ref={input} label={t('lib.saveViewName')} value={name} maxLength={200} disabled={busy} placeholder={t('lib.saveViewPlaceholder')}
      onChange={event => setName(event.currentTarget.value)} onKeyDown={event => {if (event.key === 'Enter') {event.preventDefault();save();}}}/>
    <small>{t('lib.saveViewHelp')}</small>
  </Dialog>;
}
