import { NavigationMoveDownIcon, NavigationMoveUpIcon, AccountVisibilityShowIcon, AccountVisibilityHideIcon, ActionCustomizeIcon, ActionResetIcon, ActionCloseIcon } from '#portico-icons';
import { productMessage, type ProductMessagePresentation } from '@porticomediaserver/client-core';
import { useMemo, useState } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { productLanguageProblem } from '../../components/states/ProductLanguageState';
import type { HomeRow } from '../../data/models';
import {
  completeHomeRowOrder,
  type WebDisplayPreferences,
} from '../../preferences/webDisplayPreferences';

function move<T>(items: readonly T[], index: number, direction: -1 | 1): T[] {
  const destination = index + direction;
  if (destination < 0 || destination >= items.length) return [...items];
  const next = [...items];
  [next[index], next[destination]] = [next[destination], next[index]];
  return next;
}

export function HomeCustomizationDialog({
  rows,
  preferences,
  busy,
  onDismiss,
  onSave,
}: {
  rows: HomeRow[];
  preferences: WebDisplayPreferences;
  busy: boolean;
  onDismiss: () => void;
  onSave: (preferences: WebDisplayPreferences) => Promise<void>;
}) {
  const initialOrder = useMemo(() => completeHomeRowOrder(rows, preferences.homeRowOrder), [preferences.homeRowOrder, rows]);
  const [order, setOrder] = useState(initialOrder);
  const [hidden, setHidden] = useState(() => preferences.hiddenHomeRows.filter((id) => rows.some((row) => row.id === id)));
  const [error, setError] = useState<ProductMessagePresentation>();
  const byId = useMemo(() => new Map(rows.map((row) => [row.id, row])), [rows]);
  const orderedRows = order.flatMap((id) => {
    const row = byId.get(id);
    return row ? [row] : [];
  });
  const reset = () => {
    setOrder(completeHomeRowOrder(rows, []));
    setHidden(rows.filter((row) => row.defaultVisible === false && !row.required && row.hideable === true).map((row) => row.id));
    setError(undefined);
  };
  const save = async () => {
    setError(undefined);
    try {
      const allowedHidden = hidden.filter((id) => {
        const row = byId.get(id);
        return Boolean(row && !row.required && row.hideable === true);
      });
      await onSave({ ...preferences, homeRowOrder: order, hiddenHomeRows: allowedHidden });
      onDismiss();
    } catch (reason) {
      setError(productLanguageProblem(reason, 'home.preferences-save-failed'));
    }
  };

  return <ModalOverlay labelledBy="customize-home-title" className="customize-home-dialog" onDismiss={busy ? () => undefined : onDismiss}>
    <header>
      <div><h2 id="customize-home-title">{productMessage('home.customize-title').text}</h2><p>{productMessage('home.customize-body').text}</p></div>
      <IconButton label={productMessage('action.close-home-customization').text ?? ''} disabled={busy} onClick={onDismiss}><ActionCloseIcon /></IconButton>
    </header>
    <div className="customize-home-list">
      {orderedRows.map((row, index) => {
        const visible = !hidden.includes(row.id);
        const canHide = !row.required && row.hideable === true;
		const canMove = row.reorderable === true;
        return <article key={row.id} className={visible ? '' : 'hidden'}>
          <ActionCustomizeIcon aria-hidden="true" />
          <span><strong>{row.title}</strong>{row.explanation && <small>{row.explanation}</small>}</span>
          <div>
            {canMove && <IconButton label={productMessage('action.move-row-up', { title: row.title }).text ?? ''} disabled={busy || index === 0} onClick={() => setOrder((current) => move(current, index, -1))}><NavigationMoveUpIcon /></IconButton>}
            {canMove && <IconButton label={productMessage('action.move-row-down', { title: row.title }).text ?? ''} disabled={busy || index === orderedRows.length - 1} onClick={() => setOrder((current) => move(current, index, 1))}><NavigationMoveDownIcon /></IconButton>}
            {canHide && <button type="button" className="home-row-visibility" disabled={busy} aria-pressed={visible} onClick={() => setHidden((current) => visible ? [...current, row.id] : current.filter((id) => id !== row.id))}>{visible ? <AccountVisibilityShowIcon /> : <AccountVisibilityHideIcon />}{productMessage(visible ? 'home.row-shown' : 'home.row-hidden').text}</button>}
            {!canHide && <span className="home-row-required">{productMessage('home.row-always-shown').text}</span>}
          </div>
        </article>;
      })}
    </div>
    {error && <p className="customize-home-error" role="alert">{error.body ?? error.title}</p>}
    <footer>
      <button type="button" className="home-reset-layout" disabled={busy} onClick={reset}><ActionResetIcon /> {productMessage('action.reset-layout').text}</button>
      <span><SecondaryButton disabled={busy} onClick={onDismiss}>{productMessage('action.cancel').text}</SecondaryButton><PrimaryButton disabled={busy} onClick={() => void save()}>{productMessage(busy ? 'state.saving' : 'action.save-layout').text}</PrimaryButton></span>
    </footer>
  </ModalOverlay>;
}
