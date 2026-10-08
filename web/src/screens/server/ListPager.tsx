import type {CursorRead} from '../../admin/cursor-read';
import {useI18n} from '../../app/i18n';
import {Button, Text} from '../../ui';

/** Previous / Page N / Next under a console list read by cursor (PERF-S15); nothing when it fits one page. */
export function ListPager({pages}: {pages: Pick<CursorRead<unknown>, 'page' | 'canPrevious' | 'canNext' | 'next' | 'previous' | 'loading'>}) {
  const {t} = useI18n();
  if (!pages.canPrevious && !pages.canNext) return null;
  return (
    <div style={{display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 8, padding: '8px 16px 12px'}}>
      <Text variant="caption" tone="tertiary">{t('web.pager.page', {page: pages.page})}</Text>
      <Button size="sm" variant="ghost" icon="back" label={t('web.pager.previous')} disabled={!pages.canPrevious || pages.loading} onClick={pages.previous} />
      <Button size="sm" variant="ghost" iconAfter="forward" label={t('web.pager.next')} disabled={!pages.canNext || pages.loading} onClick={pages.next} />
    </div>
  );
}
