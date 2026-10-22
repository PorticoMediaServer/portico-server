import {useEffect, useMemo, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {cardText, type CardSubject} from '@portico/client-core/kit-model';
import {createWindowedCollection} from '@core/collections/windowed';
import {AmbientArt, Artwork, createArtworkRuntime, type AmbientArtSource} from '../../kit/artwork';
import {PosterCard, WideCard, CircleCard, SquareCard, type CardContent} from '../../kit/cards';
import {Button} from '../../kit/controls';
import {SongRow, type TrackRowData} from '../../kit/browse';
import {AlphabetRail, FlowSections, FocusPreviewHero, Hero, HeroFacts, HeroProgress, Lockup, Shelf, type HeroCandidate, type HeroRenderContext, type LayoutPlatform, type WindowedGridHandle, WindowedGrid} from '../../kit/layout';
import {defineGalleryFamily} from '../gallery-registry';
import styles from '../../kit/layout/gallery.module.css';
import referenceHtml from '../../../../../../design-ui-v1/design/ui-v1/mock.html?raw';
import searchHtml from '../../../../../../design-ui-v1/design/ui-v1/desktop-search.html?raw';
import tvHtml from '../../../../../../design-ui-v1/design/ui-v1/tv.html?raw';
import manifestJson from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/manifest.json?raw';
import severance from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-sev.jpg';
import spirited from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-spirited.jpg';
import spiderverse from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-spiderverse.jpg';
import fellowship from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-lotr.jpg';
import lastOfUs from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/tv/the-last-of-us--bd.jpg';
import posterSeverance from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/tv/severance.jpg';
import posterSpirited from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/spirited-away.jpg';
import posterSpiderverse from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/spider-man-into-the-spider-verse.jpg';
import posterFellowship from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/the-lord-of-the-rings-the-fellowship-of-the-ring.jpg';
import posterLastOfUs from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/tv/the-last-of-us.jpg';
import rumours from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/albums/rumours.jpg';
import abbeyRoad from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/albums/abbey-road-2019-mix.jpg';
import mayday from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/mayday.jpg';
import matrix from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/the-matrix.jpg';
import practicalMagic from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/practical-magic-2.jpg';
import hailMary from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/project-hail-mary.jpg';
import acrossSpiderverse from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/spider-man-across-the-spider-verse.jpg';
import brandNewDay from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/spider-man-brand-new-day.jpg';
import continueLastOfUs from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-tlou.jpg';
import continueInsideOut from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-insideout.jpg';
import continueRick from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-rick.jpg';
import continueSolo from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-solo.jpg';
import tvInsideOut from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/movies/inside-out--bd.jpg';
import tvRick from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/tv/rick-and-morty--bd.jpg';
import tvSolo from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/anime/solo-leveling--bd.jpg';

// Reference content is dev-only. Production discovery emits an empty family list.
export type LayoutArtworkEvidence = Readonly<{ambients: Readonly<Record<string, AmbientArtSource>>}>;
declare global { interface Window { __porticoLayoutArtworkEvidence?: LayoutArtworkEvidence } }
/** Only current-authority sources constructed through the actual artwork runtime belong here. */
export function bindLayoutArtworkEvidence(evidence?: LayoutArtworkEvidence) {
  window.__porticoLayoutArtworkEvidence = evidence;
  window.dispatchEvent(new Event('portico-layout-artwork-evidence'));
}
function useLayoutArtworkEvidence() {
  const [evidence, setEvidence] = useState(window.__porticoLayoutArtworkEvidence);
  useEffect(() => {
    const update = () => setEvidence(window.__porticoLayoutArtworkEvidence);
    window.addEventListener('portico-layout-artwork-evidence', update);
    update();
    return () => window.removeEventListener('portico-layout-artwork-evidence', update);
  }, []);
  return evidence;
}
const reference = new DOMParser().parseFromString(referenceHtml, 'text/html');
const slides = [...reference.querySelectorAll('#hero .slide')];
type Candidate = HeroCandidate & {url: string; poster: string};
const pictures = [severance, spirited, spiderverse, fellowship], posters = [posterSeverance, posterSpirited, posterSpiderverse, posterFellowship];
const candidates: readonly Candidate[] = slides.map((slide, index) => ({
  id: `reference-${index}`, title: slide.querySelector('.lockup')!.textContent!, long: slide.querySelector('.lockup')!.classList.contains('long'),
  reason: slide.querySelector('.why')!.textContent!, overview: slide.querySelector('.blurb')!.textContent!, url: pictures[index], thumbnail: pictures[index], poster: posters[index],
  facts: [...slide.querySelectorAll('.meta > *')].map((part, i) => ({id: part.classList.contains('tag') ? index === 0 ? 'season-count' : 'duration' : `reference-fact-${i}`, text: part.textContent!, role: 'value' as const})),
}));
const t = defaultI18n.t;
const tvReference = new DOMParser().parseFromString(tvHtml, 'text/html').querySelector('#f1 .tvc')!;
const tvEntry: Candidate = {id: 'reference-tv-focused', title: tvReference.querySelector('.lockup')!.textContent!, url: lastOfUs, poster: posterLastOfUs,
  overview: tvReference.querySelector('.blurb')!.textContent!, facts: [...tvReference.querySelectorAll('.meta > :not(.mprog)')].map((part, index) => ({id: part.classList.contains('tag') ? 'season-count' : `reference-${index}`, text: part.textContent!, role: 'value'})),
};
const previewCandidates = [tvEntry, ...candidates.slice(1)];
const searchMoviePictures = [mayday, matrix, practicalMagic, hailMary, posterSpiderverse, acrossSpiderverse, brandNewDay];
const tvReason = [...tvReference.querySelector('.why')!.childNodes].filter(node => node.nodeType === Node.TEXT_NODE).map((node, index) => ({id: `reason-${index}`, text: node.textContent!}));
const tvRemaining = tvReference.querySelector('.mprog')!.textContent!, tvProgress = parseFloat((tvReference.querySelector('.mprog i') as HTMLElement).style.width) / 100;
type ShelfEntry = Readonly<{id: string; url: string; position: string; model: CardContent}>;
function referenceShelf(cards: readonly Element[], urls: readonly string[]): readonly ShelfEntry[] {
  return cards.map((card, index) => {
    const subtitle = card.querySelector('.on-art small')!, detail = card.querySelector(':scope > .s');
    return {id: `continue-${index}`, url: urls[index], position: (card.querySelector('img') as HTMLImageElement).style.objectPosition,
      model: {title: card.querySelector('.on-art b')!.textContent!,
        caption: [...subtitle.childNodes].filter(node => node.nodeType === Node.TEXT_NODE).map((node, i) => ({id: `caption-${i}`, text: node.textContent!, role: 'context' as const})),
        detail: detail ? [{id: 'remaining', text: detail.textContent!, role: 'supplement' as const}] : [],
        progress: parseFloat((card.querySelector('.prog i') as HTMLElement).style.width) / 100}};
  });
}
const desktopShelf = referenceShelf([...reference.querySelectorAll('.rows > .row:first-child .card.wide')], [continueLastOfUs, continueInsideOut, continueRick, continueSolo]);
const phoneShelf = referenceShelf([...reference.querySelector('.phone .row')!.querySelectorAll('.card.wide')], [continueLastOfUs, continueRick]);
const tvDocument = new DOMParser().parseFromString(tvHtml, 'text/html');
const tvShelf = referenceShelf([...tvDocument.querySelector('#f1 .trk')!.querySelectorAll('.card.wide')], [continueLastOfUs, tvInsideOut, tvRick, tvSolo]);

function useReferenceArtwork() {
  const evidence = useLayoutArtworkEvidence();
  const runtime = useMemo(() => createArtworkRuntime({scopeKey: 'layout-reference-gallery',
    load: async ({url}, signal) => {
      const response = await fetch(url, {signal});
      if (!response.ok) throw Object.assign(new Error('Reference artwork could not be read.'), {status: response.status});
      const blob = await response.blob(), src = URL.createObjectURL(blob), image = new Image(); image.src = src;
      try { await image.decode(); if (signal.aborted) throw signal.reason; return {src, width: image.naturalWidth, height: image.naturalHeight, bytes: image.naturalWidth * image.naturalHeight * 4}; }
      catch (error) { URL.revokeObjectURL(src); throw error; }
    }, classify: error => ({retryable: error instanceof TypeError}), release: image => URL.revokeObjectURL(image.src),
  }), []);
  useEffect(() => () => runtime.dispose(), [runtime]);
  return useMemo(() => ({runtime, backgrounds: candidates.map(item => runtime.source({url: item.url, cacheKey: item.url})), posters: candidates.map(item => runtime.source({url: item.poster, cacheKey: item.poster})), searchMovies: searchMoviePictures.map(url => runtime.source({url, cacheKey: url})), album: runtime.source({url: rumours, cacheKey: rumours}), abbeyRoad: runtime.source({url: abbeyRoad, cacheKey: abbeyRoad}), preview: runtime.source({url: lastOfUs, cacheKey: lastOfUs}),
    shelf: new Map([...desktopShelf, ...phoneShelf, ...tvShelf].map(item => [item.url, runtime.source({url: item.url, cacheKey: item.url})])), ambients: evidence?.ambients,
  }), [runtime, evidence]);
}

function LockupDesktop() { return <Lockup title={candidates[0].title} />; }
function LockupLong() { return <Lockup title={candidates[2].title} long />; }
function LockupPhone() { return <Lockup title={candidates[1].title} platform="phone-web" />; }
function LockupTv() { const title = (JSON.parse(manifestJson) as {tv: {title: string}[]}).tv.find(item => item.title.startsWith('The Last'))!.title; return <Lockup title={title} platform="tv" />; }

function HeroCase({platform = 'desktop', reduced = false}: {platform?: LayoutPlatform; reduced?: boolean}) {
  const art = useReferenceArtwork(), [index, setIndex] = useState(platform.startsWith('phone') ? 1 : 0), [saved, setSaved] = useState<ReadonlySet<string>>(new Set()), [playing, setPlaying] = useState<string | null>(null);
  const artwork = (item: Candidate, context: HeroRenderContext) => <AmbientArt source={art.ambients?.[item.id]} foreground={art.backgrounds[candidates.indexOf(item)]} treatment={context.platform.startsWith('phone') ? 'phone' : 'hero'} visible={context.visible} onReady={context.onReady} onError={context.onError} />;
  return <div data-gallery-playing={playing ?? undefined}><Hero candidates={candidates} activeIndex={index} onIndexChange={setIndex} platform={platform} reducedMotion={reduced} renderArtwork={artwork}
    renderThumbnail={(item, context) => <Artwork source={art.backgrounds[candidates.indexOf(item)]} visible={context.visible} alt="" />}
    renderActions={(item, context) => <><Button variant="primary" icon="i-play" phone={context.platform.startsWith('phone')} onClick={() => setPlaying(previous => previous === item.id ? null : item.id)} aria-pressed={playing === item.id}>
      {item === candidates[0] ? t('next.layout.playEpisode', {season: 1, episode: 1}) : t('next.layout.play')}</Button>
      <Button variant="glass" icon={saved.has(item.id) ? 'i-check' : 'i-plus'} phone={context.platform.startsWith('phone')} aria-pressed={saved.has(item.id)} onClick={() => setSaved(previous => { const next = new Set(previous); if (next.has(item.id)) next.delete(item.id); else next.add(item.id); return next; })}>{t('next.layout.myList')}</Button></>} /></div>;
}
function HeroDesktop() { return <HeroCase />; }
function HeroPhoneApp() { return <HeroCase platform="phone-app" />; }
function HeroPhoneWeb() { return <HeroCase platform="phone-web" />; }
function HeroReduced() { return <HeroCase reduced />; }

function PreviewCase() {
  const art = useReferenceArtwork(), [index, setIndex] = useState(0);
  const source = (item: Candidate) => item.id === tvEntry.id ? art.preview : art.backgrounds[candidates.indexOf(item)];
  return <div className={styles.previewFixture}><FocusPreviewHero entry={previewCandidates[index]} renderArtwork={(item, context) => <AmbientArt source={art.ambients?.[item.id]} foreground={source(item)} foregroundFrameStyle={{width: 1500, height: 900}} treatment="hero" arrivalDurationMs={0} visible={context.visible} onReady={context.onReady} onError={context.onError} />}
    renderReason={item => item.id === tvEntry.id ? tvReason.map(part => <span key={part.id}>{part.text}</span>) : item.reason}
    renderMetadata={item => <HeroFacts parts={item.facts ?? []} platform="tv">{item.id === tvEntry.id && <HeroProgress progress={tvProgress} remaining={tvRemaining} />}</HeroFacts>}
    renderSupplement={() => <div className={styles.remoteHint}><span>{t('next.layout.ok')}</span>{t('next.layout.resume')}<span>{t('next.layout.holdOk')}</span>{t('next.layout.moreOptions')}</div>} />
    <div className={styles.previewControls}><Shelf heading={t('next.layout.continueWatching')} platform="tv" items={previewCandidates} getKey={item => item.id} onItemFocus={(_item, i) => setIndex(i)} renderItem={(item, context) =>
      <WideCard artwork={source(item)} model={{title: item.title, caption: item.facts ?? []}} style={{width: 400}} onNavigate={() => setIndex(context.index)} />} /></div>
  </div>;
}

function ShelfCase({platform = 'desktop'}: {platform?: LayoutPlatform}) {
  const art = useReferenceArtwork(), [event, setEvent] = useState(''), [focus, setFocus] = useState('');
  const items = platform === 'tv' ? tvShelf : platform.startsWith('phone') ? phoneShelf : desktopShelf;
  return <div data-gallery-action={event} data-gallery-focused={focus}><Shelf heading={t('next.layout.continueWatching')} platform={platform} items={items} getKey={item => item.id} itemWidth={platform === 'tv' ? 400 : platform === 'desktop' ? 316 : 260}
    onSeeAll={() => setEvent('see-all')} onItemFocus={item => setFocus(item.id)}
    renderItem={item => <WideCard artwork={art.shelf.get(item.url)} model={item.model} position={item.position} style={{width: '100%'}}
      onNavigate={() => setEvent(`navigate:${item.id}`)} onPlay={() => setEvent(`play:${item.id}`)} onMore={() => setEvent(`more:${item.id}`)} />} /></div>;
}
function ShelfDesktop() { return <ShelfCase />; }
function ShelfPhone() { return <ShelfCase platform="phone-web" />; }
function ShelfTv() { return <ShelfCase platform="tv" />; }

function FlowCase({group = 'all'}: {group?: 'all' | 'movies' | 'small'}) {
  const art = useReferenceArtwork(), [action, setAction] = useState('');
  const search = useMemo(() => new DOMParser().parseFromString(searchHtml, 'text/html'), []);
  const people = [...search.querySelectorAll('#s-all .person:not(.artist)')].map((person, index) => ({id: `person-${index}`, initials: person.querySelector('.face')!.textContent!, title: person.querySelector('b')!.textContent!, caption: [{id: 'known-for', role: 'context' as const, text: person.querySelector('small')!.textContent!}]}));
  const artist = search.querySelector('#s-all .artist')!;
  const albumData = (JSON.parse(manifestJson) as {albums: {title: string; year: number; poster: string; artist: {name: string}}[]}).albums.find(item => item.poster.endsWith('/rumours.jpg'))!;
  const movies = [...search.querySelector('#s-all .g7')!.querySelectorAll('.card')].slice(0, 7).map((card, index) => ({id: `search-movie-${index}`, title: card.querySelector('.t')!.textContent!, caption: [{id: 'year', text: card.querySelector('.s')!.textContent!, role: 'value' as const}]}));
  const songs: readonly TrackRowData[] = [...search.querySelectorAll('#s-all .sec.songs .song')].map((song, index) => {
    const names = [...song.querySelector('small')!.childNodes].filter(node => node.nodeType === Node.TEXT_NODE).map(node => node.textContent!.trim());
    const time = /^(\d+):(\d+)$/.exec(song.querySelector('.d')!.textContent!.trim())!;
    return {id: `reference-song-${index}`, text: cardText({id: `reference-song-${index}`, kind: 'song', title: song.querySelector('b')!.textContent!, artist: names[0], album: names[1], durationSeconds: Number(time[1]) * 60 + Number(time[2])}, 'song-row', {t}), artwork: <Artwork source={index === 0 ? art.abbeyRoad : art.album} alt="" />};
  });
  const sections = [
    {id: 'movies', heading: t('next.layout.movies'), count: 7, cap: 7, itemCount: 7, itemWidth: 173.714286, fullWidth: true, content: <div className={styles.fixtureMovieGrid}>{movies.map((item, index) => <PosterCard key={item.id} context="search" style={{width: '100%'}} artwork={art.searchMovies[index]} model={item} onNavigate={() => setAction(item.id)} />)}</div>},
    {id: 'people', heading: t('next.layout.people'), count: 3, cap: 3, itemCount: 3, itemWidth: 124, content: <div className={styles.fixtureCards}>{people.map(item => <CircleCard key={item.id} context="search" covers={[]} initials={item.initials} model={item} onNavigate={() => setAction(item.id)} />)}</div>},
    {id: 'artists', heading: t('next.layout.artists'), count: 1, cap: 1, itemCount: 1, itemWidth: 124, content: <CircleCard context="search" covers={[art.album]} model={{title: artist.querySelector('b')!.textContent!, caption: [{id: 'albums', text: artist.querySelector('small')!.textContent!, role: 'value'}]}} onNavigate={() => setAction('artist')} />},
    {id: 'albums', heading: t('next.layout.albums'), count: 1, cap: 1, itemCount: 1, itemWidth: 150, content: <SquareCard context="search" artwork={art.album} model={cardText({id: 'reference-album', kind: 'album', title: albumData.title, year: albumData.year, artist: albumData.artist.name}, 'ordinary', {t})} onNavigate={() => setAction('album')} />},
    {id: 'songs', heading: t('next.layout.songs'), count: t('next.layout.itemCount', {shown: 4, total: 5}), cap: 1, itemCount: 1, itemWidth: 452, grow: true, onSeeAll: () => setAction('see-all-songs'), content: <div>{songs.map(row => <SongRow key={row.id} row={row} onPlay={id => setAction(`play:${id}`)} />)}</div>},
  ];
  return <div data-gallery-action={action}><FlowSections sections={group === 'movies' ? sections.slice(0, 1) : group === 'small' ? sections.slice(1) : sections} /></div>;
}
function FlowMovies() { return <FlowCase group="movies" />; }
function FlowSmall() { return <FlowCase group="small" />; }

const anchors = [{key: '#', index: 0}, {key: 'A', index: 2}, {key: 'B', index: 9}, {key: 'C', index: 15}, {key: 'J', index: 27}, {key: 'M', index: 34}, {key: 'R', index: 45}, {key: 'S', index: 51}, {key: 'Б', index: 7000}, {key: 'あ', index: 12000}];
function RailCase({unicode = false, phone = false}: {unicode?: boolean; phone?: boolean}) {
  const [visible, setVisible] = useState({first: 27, last: 45});
  return <div className={styles.railFixture}><AlphabetRail platform={phone ? 'phone' : 'desktop'} anchors={unicode ? anchors : anchors.slice(0, -2)} visibleRange={visible} onJump={anchor => setVisible({first: anchor.index, last: anchor.index + 6})} /></div>;
}
function RailLatin() { return <RailCase />; }
function RailUnicode() { return <RailCase unicode />; }
function RailPhone() { return <RailCase phone />; }

function GridCase({empty = false, phone = false}: {empty?: boolean; phone?: boolean}) {
  const art = useReferenceArtwork(), grid = useRef<WindowedGridHandle>(null), [visible, setVisible] = useState({first: 0, last: 20}), [action, setAction] = useState('');
  const collection = useMemo(() => createWindowedCollection({pageSize: 40, maxResidentPages: 4, maxConcurrent: 1, keyOf: (item: CardSubject) => item.id,
    fetchPage: async (start, count) => ({total: empty ? 0 : 50_000, items: empty ? [] : Array.from({length: Math.min(count, 50_000 - start)}, (_, offset) => {
      const index = start + offset, item = candidates[index % candidates.length]; return {id: `reference-${index}`, title: item.title, kind: index % candidates.length === 0 ? 'show' as const : 'movie' as const, year: Number(item.facts?.[0].text)};
    })}),
  }), [empty]);
  useEffect(() => () => collection.dispose(), [collection]);
  return <div className={styles.gridFixture} data-platform={phone ? 'phone' : 'desktop'} data-gallery-action={action}>
    <WindowedGrid ref={grid} collection={collection} label={t('next.layout.movies')} height={phone ? 528 : 844} cardWidth={phone ? 108 : 170.285714} rowHeight={phone ? 205 : 322} gap={phone ? 10 : 20} rowGap={phone ? 16 : 30} onVisibleRange={(first, last) => setVisible(previous => previous.first === first && previous.last === last ? previous : {first, last})}
      renderItem={(item, context) => <PosterCard context="grid" artwork={art.posters[context.index % candidates.length]} model={cardText(item, 'ordinary', {t})} titleLines={phone ? 1 : 2} style={{width: '100%'}} onNavigate={() => setAction(item.id)} onPlay={() => setAction(`play:${item.id}`)} onMore={() => setAction(`more:${item.id}`)} />}
      renderPlaceholder={() => <div className={styles.unloaded} aria-hidden="true" />} />
    {!empty && <AlphabetRail platform={phone ? 'phone' : 'desktop'} anchors={anchors} visibleRange={visible} onJump={anchor => grid.current?.scrollToIndex(anchor.index)} />}
  </div>;
}
function GridLarge() { return <GridCase />; }
function GridEmpty() { return <GridCase empty />; }
function GridPhone() { return <GridCase phone />; }

export default defineGalleryFamily({id: 'layout', labelKey: 'next.layout.family', cases: [
  {id: 'lockup-desktop', labelKey: 'next.layout.gallery.lockupDesktop', width: 600, height: 100, render: LockupDesktop},
  {id: 'lockup-long', labelKey: 'next.layout.gallery.lockupLong', width: 600, height: 160, render: LockupLong},
  {id: 'lockup-phone', labelKey: 'next.layout.gallery.lockupPhone', width: 350, height: 60, render: LockupPhone},
  {id: 'lockup-tv', labelKey: 'next.layout.gallery.lockupTv', width: 960, height: 140, render: LockupTv},
  {id: 'hero-desktop', labelKey: 'next.layout.gallery.heroDesktop', width: 1440, height: 720, render: HeroDesktop},
  {id: 'hero-phone-app', labelKey: 'next.layout.gallery.heroPhoneApp', width: 390, height: 560, render: HeroPhoneApp},
  {id: 'hero-phone-web', labelKey: 'next.layout.gallery.heroPhoneWeb', width: 390, height: 486, render: HeroPhoneWeb},
  {id: 'hero-reduced', labelKey: 'next.layout.gallery.heroReduced', width: 1440, height: 720, render: HeroReduced},
  {id: 'focus-preview', labelKey: 'next.layout.gallery.focusPreview', width: 1920, height: 1080, render: PreviewCase},
  {id: 'shelf-desktop', labelKey: 'next.layout.gallery.shelfDesktop', width: 1440, height: 280, render: ShelfDesktop},
  {id: 'shelf-phone', labelKey: 'next.layout.gallery.shelfPhone', width: 390, height: 250, render: ShelfPhone},
  {id: 'shelf-tv', labelKey: 'next.layout.gallery.shelfTv', width: 1920, height: 400, render: ShelfTv},
  {id: 'flow', labelKey: 'next.layout.gallery.flow', width: 1440, height: 1000, render: FlowCase},
  {id: 'flow-movies', labelKey: 'next.layout.gallery.flowMovies', width: 1440, height: 408, render: FlowMovies},
  {id: 'flow-small', labelKey: 'next.layout.gallery.flowSmall', width: 1440, height: 340, render: FlowSmall},
  {id: 'grid', labelKey: 'next.layout.gallery.grid', width: 1440, height: 844, render: GridLarge},
  {id: 'grid-phone', labelKey: 'next.layout.gallery.gridPhone', width: 390, height: 528, render: GridPhone},
  {id: 'rail', labelKey: 'next.layout.gallery.rail', width: 100, height: 620, render: RailLatin},
  {id: 'rail-unicode', labelKey: 'next.layout.gallery.railUnicode', width: 100, height: 660, render: RailUnicode},
  {id: 'rail-phone', labelKey: 'next.layout.gallery.railPhone', width: 100, height: 554, render: RailPhone},
  {id: 'empty', labelKey: 'next.layout.gallery.empty', width: 1440, height: 844, render: GridEmpty},
]});
