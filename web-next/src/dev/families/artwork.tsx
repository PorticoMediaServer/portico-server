import {useEffect, useMemo, useState, type ComponentType, type ReactNode} from 'react';
import {defaultI18n, formatMessage} from '@i18n';
import {artworkFixtures} from '../../../../packages/i18n/src/catalog/en-US/next/dev/artwork-fixtures';
import {defineGalleryFamily} from '../gallery-registry';
import {AmbientArt, Artwork, ArtistCircle, ChannelMark, ColourBackdrop, CoverMosaic, GeneratedArt, GeneratedCover, PersonCircle, ProfileFace, ProfilePicker, createArtworkRuntime, type AmbientArtSource, type ArtworkPalette, type ArtworkSource, type ProfilePickerProfile} from '../../kit/artwork';
import styles from '../../kit/artwork/gallery.module.css';
import afterHours from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/albums/after-hours.jpg?url';
import abbey from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/albums/abbey-road-2019-mix.jpg?url';
import rumours from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/albums/rumours.jpg?url';
import taylor from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/albums/1989-taylor-s-version-deluxe.jpg?url';
import severance from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/tv/severance--bd.jpg?url';
import jujutsu from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/anime/jujutsu-kaisen.jpg?url';
import spirited from '../../../../../../design-ui-v1/design/ui-v1/assets/bd-spirited.jpg?url';
import martian from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/books/the-martian-unabridged.jpg?url';
import harry from '../../../../../../design-ui-v1/design/ui-v1/assets/pool/books/harry-potter-and-the-philosopher-s-stone.jpg?url';
import nonImage from '../../kit/fonts/OFL.txt?url';

const t = defaultI18n.t;
const fixture = (id: keyof typeof artworkFixtures) => formatMessage(artworkFixtures[id]);
const palette = {first: '#2f86bb', second: '#1e5b52'};
const gold = {first: '#a8892a', second: '#5a3a1a'};
const teal = {first: '#2e6a6f', second: '#1f4f7a'};
type ArtworkEvidence = Readonly<{foreground?: ArtworkSource; ambient?: AmbientArtSource; ambients?: readonly AmbientArtSource[]; palette?: ArtworkPalette; logo?: ArtworkSource; failedLogo?: ArtworkSource}>;
declare global { var __porticoWebArtworkEvidence: ArtworkEvidence | undefined; }
/** Dev-only injection of real authority-bound handles; never credentials or fabricated ambient metadata. */
function useArtworkEvidence() {
  const [evidence, setEvidence] = useState(globalThis.__porticoWebArtworkEvidence);
  useEffect(() => {
    const update = () => setEvidence(globalThis.__porticoWebArtworkEvidence);
    window.addEventListener('portico-artwork-evidence', update);
    return () => window.removeEventListener('portico-artwork-evidence', update);
  }, []);
  return evidence;
}
function useReferenceArt() {
  const runtime = useMemo(() => createArtworkRuntime({scopeKey: 'dev-only-private-design-references',
    load: async ({url}, signal) => {
      const response = await fetch(url, {signal, credentials: 'omit', redirect: 'error'});
      if (!response.ok) throw new Error('Reference image request failed.');
      const blob = await response.blob();
      const src = URL.createObjectURL(blob), image = new Image(); image.src = src;
      try { await image.decode(); if (signal.aborted) throw new DOMException('Aborted', 'AbortError'); }
      catch (error) { URL.revokeObjectURL(src); throw error; }
      return {src, bytes: blob.size, width: image.naturalWidth, height: image.naturalHeight};
    }, classify: () => ({retryable: false}), release: image => URL.revokeObjectURL(image.src),
  }), []);
  useEffect(() => () => runtime.dispose(), [runtime]);
  const sources = useMemo(() => ({afterHours, abbey, rumours, taylor, severance, jujutsu, spirited, martian, harry, nonImage}), []);
  return useMemo(() => Object.fromEntries(Object.entries(sources).map(([name, url]) => [name, runtime.source({url, cacheKey: url})])) as Record<keyof typeof sources, ReturnType<typeof runtime.source>>, [runtime, sources]);
}
function Stage({children, className = ''}: {children: ReactNode; className?: string}) { return <div className={`${styles.stage} ${className}`}>{children}</div>; }
function ambient(treatment: 'hero' | 'title' | 'phone' | 'cover' | 'wash' | 'library' | 'artist' | 'poster', foreground = false): ComponentType {
  return function AmbientCase() { const refs = useReferenceArt(), evidence = useArtworkEvidence(); return <Stage><AmbientArt treatment={treatment} palette={evidence?.palette ?? palette} source={evidence?.ambient} sources={evidence?.ambients} foreground={foreground ? evidence?.foreground ?? (treatment === 'poster' ? refs.jujutsu : treatment === 'phone' ? refs.spirited : refs.severance) : undefined} /></Stage>; };
}
function colour(shape: 'side' | 'round' | 'top', fill = false): ComponentType { return () => <Stage><ColourBackdrop shape={shape} fill={fill} /></Stage>; }
function cover(size: 'large' | 'grid' | 'phone' | 'phone-book' | 'small' | 'long' | 'no-author'): ComponentType {
  return () => <Stage><GeneratedCover title={fixture(size === 'long' ? 'next.artwork.fixture.war' : 'next.artwork.fixture.timeMachine')} author={size === 'no-author' ? undefined : fixture('next.artwork.fixture.wells')} tint={size === 'long' ? teal : gold} glyph={size === 'long' ? 'W' : 'T'} fontScale={size === 'large' ? 17.1 : size === 'phone-book' ? 16.9 : size === 'long' ? 12 : 15.5} /></Stage>;
}
function artist(count: number): ComponentType { return function ArtistCase() { const refs = useReferenceArt(); return <Stage><ArtistCircle covers={[refs.afterHours, refs.abbey, refs.rumours, refs.taylor].slice(0, count)} alt={t('next.artwork.case.artist')} size={172} /></Stage>; }; }
function person(covers: boolean): ComponentType { return function PersonCase() { const refs = useReferenceArt(); return <Stage><PersonCircle covers={covers ? [refs.harry, refs.martian] : []} initials={covers ? 'JR' : 'MF'} alt={t('next.artwork.case.person')} size={covers ? 172 : 96} /></Stage>; }; }
function mosaic(count: number): ComponentType { return function MosaicCase() { const refs = useReferenceArt(); return <Stage><CoverMosaic covers={[refs.afterHours, refs.abbey, refs.rumours, refs.taylor].slice(0, count)} alt={t('next.artwork.case.mosaic')} /></Stage>; }; }
function channel(mode: 'monogram' | 'logo' | 'error'): ComponentType { return function ChannelCase() { const refs = useReferenceArt(), evidence = useArtworkEvidence(); return <Stage><ChannelMark mark={{sourceId: 'reference-lineup', id: 'northwind-news-24', text: 'N24', seed: 'reference-lineup:northwind-news-24'}} source={mode === 'error' ? evidence?.failedLogo ?? refs.nonImage : mode === 'logo' ? evidence?.logo : undefined} tint="#286383" alt={t('next.artwork.case.channel')} /></Stage>; }; }
const profiles: readonly ProfilePickerProfile[] = [
  {id: 'demo', name: fixture('next.artwork.fixture.demo'), eligible: true, art: 'blue', primary: true},
  {id: 'alex', name: fixture('next.artwork.fixture.alex'), eligible: true, art: 'violet', pinRequired: true},
  {id: 'kids', name: fixture('next.artwork.fixture.kids'), eligible: true, art: 'mint'},
  {id: 'sam', name: fixture('next.artwork.fixture.sam'), eligible: false, art: 'coral', unavailableReason: 'server_restriction'},
  {id: 'jules', name: fixture('next.artwork.fixture.jules'), eligible: false, art: 'gold', unavailableReason: 'membership_required'},
];
function picker(layout: 'desktop' | 'phone' | 'tv'): ComponentType {
  return function PickerCase() {
    const [outcome, setOutcome] = useState('');
    return <Stage className={`${styles.profileStage} ${layout === 'phone' ? styles.profilePhone : layout === 'tv' ? styles.profileTV : ''}`}>
      <ColourBackdrop shape={layout === 'phone' ? 'top' : 'round'} />
      <h1>{t('next.artwork.who')}</h1>{layout === 'phone' && <p>{fixture('next.artwork.fixture.server')}</p>}
      <ProfilePicker profiles={profiles} layout={layout} autoFocusId="alex" onSelect={profile => setOutcome(profile.name)} onAdd={() => setOutcome(t('next.artwork.addProfile'))} onManage={() => setOutcome(t('next.artwork.manageProfiles'))} />
      <output className={styles.outcome}>{outcome}</output>
    </Stage>;
  };
}
function Faces() { return <Stage><div className={styles.row}>{(['blue', 'violet', 'mint', 'coral', 'gold', 'slate', 'rose', 'sky'] as const).map(art => <ProfileFace key={art} initial="D" art={art} size={64} locked={art === 'violet'} selected={art === 'blue'} />)}</div></Stage>; }
function Lifecycle() {
  const refs = useReferenceArt(), [which, setWhich] = useState<'ready' | 'error' | 'empty'>('empty');
  return <Stage className={styles.lifecycle}><div className={styles.artworkBox}><Artwork source={which === 'ready' ? refs.jujutsu : which === 'error' ? refs.nonImage : undefined} arrivalDurationMs={240} alt={t('next.artwork.case.lifecycle')} fallback={<span className={styles.fallbackLabel}>{t('next.artwork.noArtwork')}</span>} /></div>
    <div className={styles.lifecycleControls}><button type="button" className={styles.button} onClick={() => setWhich('ready')}>{fixture('next.artwork.fixture.showArtwork')}</button><button type="button" className={styles.button} onClick={() => setWhich('error')}>{t('next.artwork.case.lifecycle')}</button></div>
  </Stage>;
}
const cases = [
  {id: 'ambient-hero', labelKey: 'next.artwork.case.ambient', width: 1440, height: 720, render: ambient('hero', true)},
  {id: 'ambient-title', labelKey: 'next.artwork.case.ambient', width: 1440, height: 660, render: ambient('title', true)},
  {id: 'ambient-phone', labelKey: 'next.artwork.case.ambient', width: 390, height: 560, render: ambient('phone', true)},
  {id: 'ambient-poster', labelKey: 'next.artwork.case.ambient', width: 858, height: 236, render: ambient('poster', true)},
  {id: 'ambient-palette', labelKey: 'next.artwork.case.ambient', width: 1440, height: 900, render: ambient('wash')},
  {id: 'ambient-library', labelKey: 'next.artwork.case.ambient', width: 1440, height: 620, render: ambient('library')},
  {id: 'ambient-artist', labelKey: 'next.artwork.case.ambient', width: 1440, height: 640, render: ambient('artist')},
  {id: 'ambient-cover', labelKey: 'next.artwork.case.ambient', width: 340, height: 340, render: ambient('cover', true)},
  {id: 'colour-side', labelKey: 'next.artwork.case.colour', width: 1440, height: 900, render: colour('side')},
  {id: 'colour-round', labelKey: 'next.artwork.case.colour', width: 1440, height: 900, render: colour('round')},
  {id: 'colour-top', labelKey: 'next.artwork.case.colour', width: 390, height: 844, render: colour('top')},
  {id: 'colour-full', labelKey: 'next.artwork.case.colour', width: 390, height: 844, render: colour('top', true)},
  {id: 'generated-art', labelKey: 'next.artwork.case.generated', width: 858, height: 236, render: () => <Stage className={styles.fallback}><GeneratedArt glyph="L" /></Stage>},
  ...(['large', 'grid', 'phone', 'phone-book', 'small', 'long', 'no-author'] as const).map(size => ({id: `generated-cover-${size}`, labelKey: 'next.artwork.case.cover' as const, width: size === 'large' ? 300 : size === 'phone' ? 108 : size === 'phone-book' ? 168 : size === 'small' ? 40 : 170, height: size === 'large' ? 450 : size === 'phone' ? 162 : size === 'phone-book' ? 252 : size === 'small' ? 60 : 255, render: cover(size)})),
  ...([1, 2, 3, 4] as const).map(count => ({id: `artist-${count}`, labelKey: 'next.artwork.case.artist' as const, width: 172, height: 172, render: artist(count)})),
  {id: 'person-plain', labelKey: 'next.artwork.case.person', width: 96, height: 96, render: person(false)},
  {id: 'person-covers', labelKey: 'next.artwork.case.person', width: 172, height: 172, render: person(true)},
  ...([1, 2, 4] as const).map(count => ({id: `mosaic-${count}`, labelKey: 'next.artwork.case.mosaic' as const, width: 190, height: 190, render: mosaic(count)})),
  {id: 'channel-monogram', labelKey: 'next.artwork.case.channel', width: 34, height: 34, render: channel('monogram')},
  {id: 'channel-logo', labelKey: 'next.artwork.case.channel', width: 34, height: 34, render: channel('logo')},
  {id: 'channel-logo-error', labelKey: 'next.artwork.case.channel', width: 34, height: 34, render: channel('error')},
  {id: 'profile-faces', labelKey: 'next.artwork.case.profile', width: 760, height: 112, render: Faces},
  {id: 'profile-picker-desktop', labelKey: 'next.artwork.case.profile', width: 1440, height: 900, render: picker('desktop')},
  {id: 'profile-picker-phone', labelKey: 'next.artwork.case.profile', width: 390, height: 844, render: picker('phone')},
  {id: 'profile-picker-tv', labelKey: 'next.artwork.case.profile', width: 1920, height: 1080, render: picker('tv')},
  {id: 'artwork-lifecycle', labelKey: 'next.artwork.case.lifecycle', width: 390, height: 420, render: Lifecycle},
] as const;
export default defineGalleryFamily({id: 'artwork', labelKey: 'next.artwork.family', cases});
