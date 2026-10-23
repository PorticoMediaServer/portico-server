import {Fragment, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import type {DetailCredit, DetailProjection} from '@core/detail.ts';
import {PersonCircle} from '../../kit/artwork';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays';
import type {DetailBindings} from './bindings';
import {t, locale} from './text';
import styles from './detail.module.css';
import {runScreenCommand} from './context';
import {WindowedGrid} from '../../kit/layout';
import {titleCreditsWindow} from './credits-window';

function Person({credit, binding, data}: {credit: DetailCredit; binding: DetailBindings; data: DetailProjection}) {
  const art = binding.context.artwork.foreground(credit.portraitUrl, 208, 1);
  return <button type="button" disabled={!credit.personId} onClick={() => runScreenCommand(binding.context, () => {
    binding.current(data); if (credit.personId) binding.context.effects.openEntry({id: credit.personId, kind: 'person', title: credit.name, navigation: {view: 'person', entityId: credit.personId}});
  })}><PersonCircle covers={art ? [art] : []} initials={credit.name.split(/\s+/).slice(0, 2).map(word => word[0]).join('')} alt="" /><b>{credit.name}</b>{credit.role && <small>{credit.role}</small>}</button>;
}
const castCredit = (credit: DetailCredit) => credit.department.toLowerCase() === 'acting' || credit.department.toLowerCase() === 'cast';
export function CreditNames({binding, data, credits}: {binding: DetailBindings; data: DetailProjection; credits: readonly DetailCredit[]}) {
  let index = 0;
  return <>{new Intl.ListFormat(locale(), {type: 'unit', style: 'long'}).formatToParts(credits.map(credit => credit.name)).map((part, position) => {
    if (part.type === 'literal') return <Fragment key={position}>{part.value}</Fragment>;
    const credit = credits[index++];
    return credit.personId ? <button className={styles.inlineLink} key={position} type="button" onClick={() => runScreenCommand(binding.context, () => { binding.current(data); binding.context.effects.openEntry({id: credit.personId!, kind: 'person', title: credit.name, navigation: {view: 'person', entityId: credit.personId!}}); })}>{credit.name}</button> : <Fragment key={position}>{credit.name}</Fragment>;
  })}</>;
}
/** A controlled translation slot keeps the exact catalogue sentence around its live links. */
export function CreditLine({message, ...props}: {message: 'title.directedBy' | 'title.starring'; binding: DetailBindings; data: DetailProjection; credits: readonly DetailCredit[]}) {
  const [before, after] = t(message, {names: '\uFFF0'}).split('\uFFF0');
  return <span>{before}<CreditNames {...props} />{after}</span>;
}
export function TitlePeople({binding, data, episode = false}: {binding: DetailBindings; data: DetailProjection; episode?: boolean}) {
  const [open, setOpen] = useState(false), trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => setOpen(false), [data]);
  const cast = data.metadata.credits.filter(castCredit), crew = data.metadata.credits.filter(credit => !castCredit(credit));
  const totals = data.metadata.creditTotals, total = totals ? totals.cast + totals.crew : undefined;
  const rendered = Math.min(6, cast.length) + Math.min(6, crew.length);
  return <div className={styles.creditGroups} data-episode={episode || undefined}>
    {!episode && <header className={styles.peopleHeader}><h2>{t('title.allPeople')}</h2>{total !== undefined && total > rendered && <Button ref={trigger} variant="text" size="small" icon="i-right" onClick={() => runScreenCommand(binding.context, () => { binding.current(data); setOpen(true); })}>{t('shelf.seeAllCount', {count: total})}</Button>}</header>}
    {([['cast', cast], ['crew', crew]] as const).map(([kind, credits]) => {
      if (!credits.length) return null;
      return <section key={kind}>{episode && <header className={styles.peopleHeader}><h2>{t(kind === 'cast' ? 'title.cast' : 'title.crew')}</h2>
        {totals && totals[kind] > Math.min(6, credits.length) && <Button ref={trigger} variant="text" size="small" onClick={() => runScreenCommand(binding.context, () => { binding.current(data); setOpen(true); })}>{t('shelf.seeAllCount', {count: totals[kind]})}</Button>}</header>}
        <div className={styles.people}>{credits.slice(0, 6).map(credit => <Person key={credit.id} credit={credit} data={data} binding={binding} />)}</div>
      </section>;
    })}
    <Dialog open={open} onOpenChange={setOpen} title={t('title.allPeople')} description={data.item.title} width={560} returnFocusRef={trigger}>
      {open && (['cast', 'crew'] as const).filter(group => (totals?.[group] ?? (group === 'cast' ? cast.length : crew.length)) > 0).map(group => <CreditWindow key={group} binding={binding} data={data} group={group} />)}
    </Dialog>
  </div>;
}

function CreditWindow({binding, data, group}: {binding: DetailBindings; data: DetailProjection; group: 'cast' | 'crew'}) {
  const collection = useMemo(() => titleCreditsWindow(binding, data, group), [binding, data, group]);
  const snapshot = useSyncExternalStore(collection.subscribe, collection.getSnapshot, collection.getSnapshot);
  useEffect(() => () => collection.dispose(), [collection]);
  const label = t(group === 'cast' ? 'title.cast' : 'title.crew');
  return <section className={styles.allCredits}><h3>{label}</h3>{snapshot.status === 'error' && <Button size="compact" onClick={() => collection.retry()}>{t('next.states.tryAgain')}</Button>}
    <WindowedGrid collection={collection} label={label} cardWidth={120} rowHeight={180} height={240} gap={20} rowGap={16}
      renderPlaceholder={index => <span className={styles.creditPlaceholder} key={index} />} renderItem={credit => <div className={styles.people}><Person credit={credit} binding={binding} data={data} /></div>} />
  </section>;
}
