import {useCallback, useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {useSearch} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {dvrMessage} from '@core/dvr.ts';
import {router} from '../../router';
import {Button, Segmented} from '../../kit/controls';
import {Banner, Dialog} from '../../kit/overlays';
import {Skeleton, StateView} from '../../kit/states';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {ScreenProblem} from '../detail/ScreenProblem';
import type {ScreenContext} from '../detail/context';
import {SeriesRules} from './SeriesRules';
import {createSeriesRulesBindings, type SeriesRulesBindings} from './series-rules-bindings';
import type {RuleSettings, SeriesRule} from './series-rules-model';
import {parseLiveScreenSearch} from './route-search';
import styles from '../../kit/dvr/rules.module.css';

const t = defaultI18n.t;
function MoreRules({binding, cursor}: {binding: SeriesRulesBindings; cursor: string}) {
  const element = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!element.current) return;
    const observer = new IntersectionObserver(entries => {if (entries.some(entry => entry.isIntersecting)) void binding.more().catch(() => {});});
    observer.observe(element.current);return () => observer.disconnect();
  }, [binding, cursor]);
  return <div ref={element}><Skeleton width="100%" height={114}/></div>;
}

function BoundRules({binding, context, onView}: {binding: SeriesRulesBindings; context: ScreenContext; onView?: (view: 'upcoming' | 'recorded' | 'rules' | 'history' | 'storage') => void}) {
  const state = useSyncExternalStore(binding.subscribe, binding.getSnapshot, binding.getSnapshot);
  const search = parseLiveScreenSearch(useSearch({from: '/live'}));
  const [deleting, setDeleting] = useState<Readonly<{rule: SeriesRule; resolve: (success: boolean) => void}>>(), [future, setFuture] = useState<'keep' | 'cancel'>('keep');
  const deleteResolve = useRef<((success: boolean) => void) | undefined>(undefined);
  const previewTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => {clearTimeout(previewTimer.current);deleteResolve.current?.(false);}, [binding]);
  const onDraft = useCallback((rule: SeriesRule | undefined, settings: RuleSettings | undefined) => {
    clearTimeout(previewTimer.current);binding.cancelPreview();
    if (rule && settings) previewTimer.current = setTimeout(() => {void binding.preview(rule, settings).catch(context.effects.reportError);}, 250);
  }, [binding, context]);
  const onSave = useCallback((rule: SeriesRule, settings: RuleSettings) => binding.save(rule, settings).catch(error => {context.effects.reportError(error);return false;}), [binding, context]);
  const onGuide = () => {if (context.isCurrent()) void router.navigate({to: '/live', search: {...search, tab: 'guide'}}).catch(context.effects.reportError);};
  if (!context.isCurrent()) return null;
  if (!state.view) return state.error ? <ScreenProblem context={context} error={state.error} retry={() => {void binding.load().catch(() => {});}}/>
    : <Skeleton width="100%" height={114}/>;
  const busy = state.mutating || state.phase === 'loading' || state.loadingMore;
  const remove = async () => {
    if (!deleting || busy) return;
    if (await binding.remove(deleting.rule, future).catch(error => {context.effects.reportError(error);return false;})) {deleting.resolve(true);deleteResolve.current = undefined;setDeleting(undefined);}
  };
  const closeDelete = () => {if (!busy) {deleting?.resolve(false);deleteResolve.current = undefined;setDeleting(undefined);}};
  return <><SeriesRules view={state.view} onGuide={onGuide} busy={busy} onSave={onSave} onDraft={onDraft}
    onDelete={rule => new Promise<boolean>(resolve => {deleteResolve.current?.(false);deleteResolve.current = resolve;setDeleting({rule, resolve});setFuture('keep');})} preview={state.preview}
    {...(onView ? {onView} : {})}
    error={state.error || state.previewError ? dvrMessage(state.error ?? state.previewError) : undefined}
    tail={<>{state.nextCursor && !state.error && <MoreRules binding={binding} cursor={state.nextCursor}/>}
      {state.error !== undefined && <Banner action={{label: t('next.states.tryAgain'), onAction: () => {void binding.load().catch(() => {});}}}>{dvrMessage(state.error)}</Banner>}
      {!state.view.rules.length && <StateView title={t('web.dvr.empty.rules')} body={t('web.dvr.empty.rulesBody')} primary={{id: 'guide', label: t('web.dvr.openGuide'), onPress: onGuide}}/>}</>}/>
    <Dialog open={!!deleting} onOpenChange={open => {if (!open) closeDelete();}}
      title={t('web.dvr.deleteRuleTitle', {name: deleting?.rule.title ?? ''})} description={t('web.dvr.deleteRuleBody')} width={440}
      footer={<><Button variant="glass" disabled={busy} onClick={closeDelete}>{t('action.cancel')}</Button>
        <Button className={styles.delete} variant="ghost" loading={busy} onClick={() => {void remove();}}>{t('web.dvr.deleteRuleAction')}</Button></>}>
      {state.error !== undefined && <Banner>{dvrMessage(state.error)}</Banner>}<Segmented aria-label={t('web.dvr.upcomingChoice')} disabled={busy} value={future} onValueChange={value => {if (value === 'keep' || value === 'cancel') setFuture(value);}}
        items={[{value: 'keep', label: t('web.dvr.keepUpcoming')}, {value: 'cancel', label: t('web.dvr.cancelUpcoming')}]}/>
    </Dialog></>;
}

/** No fixture imports or independent transport. This is mounted only by the
 * authenticated Live route and retires every DVR request with that scope. */
export function SeriesRulesEntry({captured, onView}: {captured: ProtectedLibraryScreen; onView?: (view: 'upcoming' | 'recorded' | 'rules' | 'history' | 'storage') => void}) {
  const context = useLibraryScreenContext(captured);
  const [owner, setOwner] = useState<Readonly<{context: ScreenContext; binding: SeriesRulesBindings}>>();
  useEffect(() => {
    const binding = createSeriesRulesBindings(context, {locale: defaultI18n.region.formatLocale,
      timezone: defaultI18n.region.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone});
    setOwner({context, binding});void binding.load().catch(() => {});
    return () => binding.dispose();
  }, [context]);
  return owner && owner.context === context ? <BoundRules binding={owner.binding} context={context} {...(onView ? {onView} : {})}/> : <Skeleton width="100%" height={114}/>;
}
