import {useEffect, useRef, useState} from 'react';
import type {CreditPage, DetailCredit, DetailProjection} from '@core/detail.ts';
import {PersonCircle} from '../../kit/artwork';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays';
import type {DetailBindings} from './bindings';
import {t} from './text';
import styles from './detail.module.css';

function Person({credit, binding, data}: {credit: DetailCredit; binding: DetailBindings; data: DetailProjection}) {
  const art = binding.context.artwork.foreground(credit.portraitUrl, 208, 1);
  return <button type="button" disabled={!credit.personId} onClick={() => {
    binding.current(data); if (credit.personId) binding.context.effects.openEntry({id: credit.personId, kind: 'person', title: credit.name, navigation: {view: 'person', entityId: credit.personId}});
  }}><PersonCircle covers={art ? [art] : []} initials={credit.name.split(/\s+/).slice(0, 2).map(word => word[0]).join('')} alt="" /><b>{credit.name}</b>{credit.role && <small>{credit.role}</small>}</button>;
}
const castCredit = (credit: DetailCredit) => credit.department.toLowerCase() === 'acting' || credit.department.toLowerCase() === 'cast';
export function TitlePeople({binding, data, episode = false}: {binding: DetailBindings; data: DetailProjection; episode?: boolean}) {
  const [group, setGroup] = useState<'cast' | 'crew'>(), [page, setPage] = useState<CreditPage>(), [failed, setFailed] = useState(false);
  const controller = useRef<AbortController | undefined>(undefined), trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => { setGroup(undefined); setPage(undefined); return () => controller.current?.abort(); }, [data]);
  const load = (which: 'cast' | 'crew', cursor?: string) => {
    controller.current?.abort(); const request = new AbortController(); controller.current = request;
    setGroup(which); setFailed(false);
    void binding.readCredits(data, which, cursor, request.signal).then(value => {
      if (request.signal.aborted) return;
      setPage(before => cursor && before?.group === value.group ? {...value, credits: [...before.credits, ...value.credits]} : value);
    }, () => { if (!request.signal.aborted) setFailed(true); });
  };
  const cast = data.metadata.credits.filter(castCredit), crew = data.metadata.credits.filter(credit => !castCredit(credit));
  return <div className={styles.creditGroups} data-episode={episode || undefined}>
    {([['cast', cast], ['crew', crew]] as const).map(([kind, credits]) => {
      if (!credits.length) return null;
      const total = data.metadata.creditTotals?.[kind];
      return <section key={kind}><header className={styles.peopleHeader}><h2>{t(kind === 'cast' ? 'title.cast' : 'title.crew')}</h2>
        {total !== undefined && total > Math.min(6, credits.length) && <Button ref={trigger} variant="text" size="small" onClick={() => load(kind)}>{t('title.seeAll')} {total}</Button>}</header>
        <div className={styles.people}>{credits.slice(0, 6).map(credit => <Person key={credit.id} credit={credit} data={data} binding={binding} />)}</div>
      </section>;
    })}
    <Dialog open={!!group} onOpenChange={open => { if (!open) { controller.current?.abort(); setGroup(undefined); setPage(undefined); } }} title={t(group === 'crew' ? 'title.allCrew' : 'title.allCast')} returnFocusRef={trigger}>
      {failed && <Button size="compact" onClick={() => group && load(group, page?.nextCursor)}>{t('next.states.tryAgain')}</Button>}
      <div className={styles.people}>{page?.credits.map(credit => <Person key={credit.id} credit={credit} data={data} binding={binding} />)}</div>
      {page?.nextCursor && <Button size="compact" onClick={() => group && load(group, page.nextCursor)}>{t(group === 'crew' ? 'title.allCrew' : 'title.moreCast')}</Button>}
    </Dialog>
  </div>;
}
