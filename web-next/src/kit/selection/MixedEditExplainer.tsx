import type {RefObject} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../controls';
import {BottomSheet, Popover} from '../overlays';
import {usePhone} from '../overlays/shared';
import {selectionText as text} from './copy';
import {selectionCountText} from './presentation';
import type {SelectionMediaKind, SelectionSurface, SelectionTypeCount} from './types';
import styles from './selection.module.css';

export type MixedEditExplainerProps = SelectionSurface & Readonly<{anchorRef: RefObject<HTMLElement | null>; types: readonly SelectionTypeCount[]; reason?: 'mixed-types' | 'no-bulk-fields'; onKeepOnly: (kind: SelectionMediaKind) => void}>;

export function MixedEditExplainer({types, onKeepOnly, anchorRef, reason = 'mixed-types', ...surface}: MixedEditExplainerProps) {
  const phone = usePhone(), total = types.reduce((sum, type) => sum + type.count, 0);
  const title = text('next.selection.editOneType');
  const children = <div className={styles.mixedBody}>
    {!phone && <h4>{title}</h4>}
    <p>{reason === 'no-bulk-fields' ? text('next.selection.noSeasonFields') : text('next.selection.mixedExplanation', {types: defaultI18n.list(types.map(selectionCountText))})}</p>
    {reason === 'mixed-types' && <><div className={styles.keepLabel}>{text('next.selection.keepOnly')}</div>
      <div className={styles.keepTypes}>{types.map(type => <Button key={type.kind} variant={phone ? 'text' : 'glass'} size="small" className={styles.keepType} onClick={() => onKeepOnly(type.kind)}>{selectionCountText(type)}{phone && <small>{text('next.selection.countOf', {count: type.count, total})}</small>}</Button>)}</div></>}
    {phone && <Button size="compact" className={styles.mixedClose} onClick={() => surface.onOpenChange(false, 'action')}>{text('next.selection.close')}</Button>}
  </div>;
  return phone ? <BottomSheet {...surface} returnFocusRef={surface.returnFocusRef ?? anchorRef} title={title} fitContent className={styles.mixedSheet}>{children}</BottomSheet> : <Popover {...surface} anchorRef={anchorRef} title={title} accessibilityLabel={text('next.selection.editNeedsType')} width={420} placement="bottom-start" className={styles.mixedPopover}>{children}</Popover>;
}
