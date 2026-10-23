import {t, locale, environment as env} from './text';
import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import type {PersonPage, PersonCredit} from '@core/people.ts';
import {Chip, Button} from '../../kit/controls';
import {AmbientArt, PersonCircle} from '../../kit/artwork';
import {PosterCard} from '../../kit/cards';
import {WindowedGrid, Lockup} from '../../kit/layout';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {DetailLoading, ScreenFailure, TitleRow} from './Detail';
import {entryCard} from './model';
import type {PersonBindings} from './person-bindings';
import styles from './person.module.css';
import {TitleProse} from '../../kit/title/TitleParts';
import {ScreenProblem} from './ScreenProblem';
import {runScreenCommand} from './context';

const date = (value: string) => /^\d{4}-\d{2}-\d{2}$/.test(value) && Number.isFinite(Date.parse(value)) ? new Intl.DateTimeFormat(locale(), {year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC'}).format(new Date(`${value}T12:00:00Z`)) : undefined;
export type PersonScreenProps = {binding: PersonBindings; personId: string};
export function PersonScreen({binding, personId}: PersonScreenProps) {
  useEffect(() => { void binding.select(personId).catch(binding.context.effects.reportError); return () => binding.cancel(); }, [binding, personId]);
  const snapshot = useSyncExternalStore(binding.subscribe, binding.getSnapshot, binding.getSnapshot);
  const [full, setFull] = useState(false), width = useShellWidth(), phone = width < 1024;
  useEffect(() => setFull(false), [personId]);
  if (!snapshot.page) return snapshot.phase === 'error' ? <ScreenProblem context={binding.context} error={snapshot.error} retry={() => { void binding.retry(); }} /> : <DetailLoading />;
  const page = snapshot.page, {person} = page, art = binding.context.artwork;
  const initials = person.name.split(/\s+/).filter(Boolean).slice(0, 2).map(word => Array.from(word)[0]).join('');
  const born = date(person.birthDate), died = date(person.deathDate);
  const covers = person.knownFor.slice(0, 4).flatMap(entry => { const source = art.foreground(entry.posterUrl, 320, 2 / 3); return source ? [source] : []; });
  return <article className={styles.person}>
    <header className={styles.hero}>
      <AmbientArt className={styles.wash} treatment="artist" sources={person.knownFor.slice(0, 3).flatMap(entry => { const source = art.ambient(entry.backdropUrl ?? entry.posterUrl); return source ? [source] : []; })} />
      <div className={styles.copy}>{person.roles.length > 0 && <p className={styles.roles}>{person.roles.join(', ')}</p>}<Lockup title={person.name} platform={phone ? 'phone-web' : 'desktop'} />
        {(born || person.birthPlace) && <p className={styles.birth}>{born && <span>{died ? t('web.person.lived', {born, died}) : t('web.person.born', {date: born})}</span>}{person.birthPlace && <span>{person.birthPlace}</span>}</p>}
        {person.biography ? <><TitleProse className={styles.biography} data-full={full || undefined}>{person.biography}</TitleProse><Button variant="text" size="small" onClick={() => runScreenCommand(binding.context, () => { binding.current(page); setFull(value => !value); })}>{t(full ? 'web.person.showLess' : 'web.person.readMore')}</Button></> : <TitleProse className={styles.biography}>{t('web.person.noBio')}</TitleProse>}
        {person.biographySource && <small className={styles.source}>{person.biographySource}</small>}
      </div><div className={styles.portrait}><PersonCircle covers={covers} initials={initials} alt={person.name} /></div>
    </header>
    {snapshot.role === 'all' && person.knownFor.length > 0 && page.pageInfo.total > person.knownFor.length && <TitleRow binding={binding} heading={t('web.person.knownFor')} entries={person.knownFor} guard={() => binding.current(page)} />}
    <div className={styles.toolbar}><span className={styles.count}>{t('web.person.titles', {count: page.pageInfo.total})}</span>{(['all', 'cast', 'crew'] as const).map(role => <Chip key={role} selected={snapshot.role === role} onClick={() => runScreenCommand(binding.context, () => { binding.current(page); return binding.select(person.id, role); })}>{t(role === 'all' ? 'web.search.all' : role === 'cast' ? 'web.person.cast' : 'web.person.crew')}</Chip>)}</div>
    {page.pageInfo.total === 0 && <StateView title={t('web.person.noCredits')} body={t('web.person.noCreditsBody')} />}
    {page.groups.length > 0 && snapshot.role === 'all' ? page.groups.filter(group => group.kind).map(group => <PersonCredits key={group.id} binding={binding} page={page} kind={group.kind} heading={t(group.kind === 'movie' ? 'search.chip.movies' : group.kind === 'show' ? 'search.chip.shows' : 'web.person.credits')} />)
      : page.pageInfo.total > 0 && <PersonCredits binding={binding} page={page} heading={t('web.person.credits')} />}
  </article>;
}
function PersonCredits({binding, page, kind, heading}: {binding: PersonBindings; page: PersonPage; kind?: string; heading: string}) {
  const collection = useMemo(() => binding.credits(page, kind), [binding, page, kind]);
  const state = useSyncExternalStore(collection.subscribe, collection.getSnapshot, collection.getSnapshot);
  const total = kind ? page.groups.find(group => group.kind === kind)?.total : page.pageInfo.total;
  return <section className={styles.group}><h2>{heading}{total !== undefined && <span className={styles.count}>{total}</span>}</h2>{state.status === 'error' && <ScreenFailure retry={() => collection.retry()} />}<WindowedGrid collection={collection} label={heading} cardWidth={190} rowHeight={350} height={kind ? 420 : 'calc(100vh - 240px)'} gap={20}
    renderPlaceholder={index => <span className={styles.placeholder} key={index} />} renderItem={(credit: PersonCredit) => <PosterCard model={entryCard(credit.media, {t, locale: locale()})} context="detail"
      artwork={binding.context.artwork.foreground(credit.media.posterUrl, 380, 2 / 3)} watched={credit.media.watched} extraCaption={[credit.roles.map((role, index) => ({id: String(index), text: role.role, role: 'context' as const}))]}
      onNavigate={() => runScreenCommand(binding.context, () => binding.open(page, credit))} />} /></section>;
}
