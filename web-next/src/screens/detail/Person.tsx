import {t, locale} from './text';
import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import type {PersonPage, PersonCredit} from '@core/people.ts';
import {Chip, Button} from '../../kit/controls';
import {Icon} from '../../kit/Icon';
import {AmbientArt, PersonCircle} from '../../kit/artwork';
import {PosterCard} from '../../kit/cards';
import {WindowedGrid, Lockup} from '../../kit/layout';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import type {CardTextPresentation} from '../../kit/cards/types';
import {DetailLoading, ScreenFailure, TitleRow} from './Detail';
import {entryCard} from './model';
import {creditRole, type PersonBindings} from './person-bindings';
import styles from './person.module.css';
import {TitleProse} from '../../kit/title/TitleParts';
import {ScreenProblem} from './ScreenProblem';
import {runScreenCommand} from './context';

const date = (value: string) => /^\d{4}-\d{2}-\d{2}$/.test(value) && Number.isFinite(Date.parse(value)) ? new Intl.DateTimeFormat(locale(), {year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC'}).format(new Date(`${value}T12:00:00Z`)) : undefined;
const groupHeading = (kind: string | undefined) => t(kind === 'movie' ? 'search.chip.movies' : kind === 'show' ? 'search.chip.shows' : 'web.person.credits');

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
  const covers = person.knownFor.length > 0 ? Array.from({length: 4}, (_, index) => person.knownFor[index % person.knownFor.length]).flatMap(entry => { const source = art.foreground(entry.posterUrl, 320, 2 / 3); return source ? [source] : []; }) : [];
  const titles = page.pageInfo.total;
  const hasMeta = Boolean(born || person.birthPlace || titles > 0);
  const hasBiography = Boolean(person.biography);
  return <article className={styles.person} data-person-screen>
    <header className={styles.hero} data-person-hero>
      <AmbientArt className={styles.wash} treatment="artist" sources={person.knownFor.slice(0, 3).flatMap(entry => { const source = art.ambient(entry.backdropUrl ?? entry.posterUrl); return source ? [source] : []; })} />
      <div className={styles.portrait} data-person-disc><PersonCircle covers={covers} initials={initials} alt={person.name} initialsSize={phone ? 44 : 128} /></div>
      <div className={styles.copy} data-person-copy>
        {person.roles.length > 0 && <p className={styles.roles} data-person-roles>{person.roles.join(', ')}</p>}
        <Lockup title={person.name} platform={phone ? 'phone-web' : 'desktop'} />
        {hasMeta && <p className={styles.meta} data-person-meta>
          {born && <strong>{died ? t('web.person.lived', {born, died}) : t('web.person.born', {date: born})}</strong>}
          {person.birthPlace && <span>{person.birthPlace}</span>}
          {titles > 0 && <span>{t('web.person.titles', {count: titles})}</span>}
        </p>}
        {hasBiography ? <TitleProse className={styles.biography} data-person-bio data-full={full || undefined}>{person.biography}</TitleProse> : <TitleProse className={styles.biography} data-person-bio>{t('web.person.noBio')}</TitleProse>}
        {(hasBiography || person.biographySource) && <div className={styles.bioFoot} data-person-bio-foot>
          {hasBiography && <Button variant="text" size="small" className={styles.readMore} onClick={() => runScreenCommand(binding.context, () => { binding.current(page); setFull(value => !value); })}>{t(full ? 'web.person.showLess' : 'web.person.readMore')}<Icon name={full ? 'i-up' : 'i-down'} size={16} /></Button>}
          {person.biographySource && <small className={styles.source} data-person-source>{person.biographySource}</small>}
        </div>}
      </div>
    </header>
    {snapshot.role === 'all' && person.knownFor.length > 0 && titles > person.knownFor.length && <TitleRow binding={binding} heading={t('web.person.knownFor')} entries={person.knownFor} guard={() => binding.current(page)} />}
    <div className={styles.toolbar} data-person-toolbar><span className={styles.count} data-person-count>{t('web.person.titles', {count: titles})}</span><div className={styles.chips}>{(['all', 'cast', 'crew'] as const).map(role => <Chip key={role} variant="quick" selected={snapshot.role === role} onClick={() => runScreenCommand(binding.context, () => { binding.current(page); return binding.select(person.id, role); })}>{t(role === 'all' ? 'web.search.all' : role === 'cast' ? 'web.person.cast' : 'web.person.crew')}</Chip>)}</div></div>
    {titles === 0 && <StateView title={t('web.person.noCredits')} body={t('web.person.noCreditsBody')} />}
    {page.groups.some(group => group.kind && group.total > 0) && snapshot.role === 'all'
      ? <div className={styles.groups} data-person-groups>{page.groups.filter(group => group.kind && group.total > 0).map(group => <PersonCredits key={group.id} binding={binding} page={page} kind={group.kind} total={group.total} heading={groupHeading(group.kind)} phone={phone} />)}</div>
      : titles > 0 && <PersonCredits binding={binding} page={page} total={titles} heading={t('web.person.credits')} phone={phone} />}
  </article>;
}

function PersonCredits({binding, page, kind, total, heading, phone}: {binding: PersonBindings; page: PersonPage; kind?: string; total: number; heading: string; phone: boolean}) {
  const collection = useMemo(() => binding.credits(page, kind), [binding, page, kind]);
  const state = useSyncExternalStore(collection.subscribe, collection.getSnapshot, collection.getSnapshot);
  const columns = Math.max(1, Math.min(total, 4));
  const grouped = kind !== undefined;
  const cardWidth = 190, gap = 16, rowHeight = 354;
  const captions = (credit: PersonCredit) => credit.roles.length > 0
    ? credit.roles.map((role, index) => creditRole(role, `role-${index}`))
    : [];
  return <section className={styles.group} data-person-group={kind ?? 'all'}>
    <h2>{heading}{total !== undefined && <span className={styles.groupCount}>{total}</span>}</h2>
    {state.status === 'error' && <ScreenFailure retry={() => collection.retry()} />}
    <WindowedGrid className={grouped ? styles.grid : styles.wideGrid} collection={collection} label={heading} cardWidth={cardWidth} rowHeight={rowHeight} gap={gap} height={grouped ? rowHeight : 'calc(100vh - 240px)'} style={grouped && !phone ? {width: columns * cardWidth + (columns - 1) * gap} : undefined}
      renderPlaceholder={index => <span className={styles.placeholder} key={index} />}
      renderItem={(credit: PersonCredit) => {
        const model = entryCard(credit.media, {t, locale: locale()}), roles = captions(credit);
        const extraCaption = roles.map(role => [role.part]);
        const extraPresentation = roles.map(role => role.presentation);
        const textPresentation: CardTextPresentation | undefined = extraPresentation.some(Boolean) ? {extraCaption: extraPresentation} : undefined;
        return <PosterCard model={{title: model.title, caption: [], progress: model.progress}} context="detail" textPresentation={textPresentation}
          artwork={binding.context.artwork.foreground(credit.media.posterUrl, 380, 2 / 3)} watched={credit.media.watched} extraCaption={extraCaption}
          onNavigate={() => runScreenCommand(binding.context, () => binding.open(page, credit))} />;
      }} />
  </section>;
}
