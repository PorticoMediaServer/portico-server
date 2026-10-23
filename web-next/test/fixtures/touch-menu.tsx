// Component visual fixture only. These drawn reference facts grant no product action authority.
import {createRoot} from 'react-dom/client';
import {useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {PosterCard} from '../../src/kit/cards/MediaCards';
import {IconSprite} from '../../src/kit/Icon';
import {ServerMark} from '../../src/kit/shell/ServerIdentity';
import {Menu, TouchMenuOwner, TouchMenuTitle, type MenuEntry} from '../../src/kit/overlays/Popover';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

const t = defaultI18n.t;
const compact = new URLSearchParams(location.search).get('case') !== 'anchored';
const referenceOrigin = new URLSearchParams(location.search).get('reference');
const selecting = new URLSearchParams(location.search).has('selecting');
if (!referenceOrigin) throw new Error('A checked reference origin is required.');
const reference = compact ? {title: 'Toy Story', year: 1995, art: 'toy-story.jpg'} : {title: 'Alien', year: 1979, art: 'alien.jpg'};
const events: string[] = [];

function TouchMenuReference() {
  const [open, setOpen] = useState(false), anchor = useRef<HTMLButtonElement | null>(null);
  const [navigations, setNavigations] = useState(0), [menus, setMenus] = useState(0), [toggles, setToggles] = useState(0), [picked, setPicked] = useState(false);
  const action = (id: string, label: string, icon: Exclude<MenuEntry, {separator: true} | {section: true}>['icon'], destructive = false): MenuEntry =>
    ({id, label, icon, destructive, onSelect: () => events.push(id)});
  // All rows in select17/tablet9 are supplied to exercise the renderer, including the
  // native-only Download reference row. This is not web product menu eligibility.
  const items: MenuEntry[] = [
    action('play', t('entry.play'), 'i-play'),
    action('watchlist', t('entry.addToWatchlist'), 'i-plus'),
    action('watched', t(compact ? 'entry.markWatched' : 'entry.markUnwatched'), 'i-check'),
    action('favorite', t('next.overlays.favorite'), 'i-heart'),
    action('rate', t('entry.rate'), 'i-star'),
    action('playlist', t('next.overlays.playlist'), 'i-listadd'),
    action('collection', t('next.overlays.collection'), 'i-layers'),
    action('download', t('next.overlays.download'), 'i-download'),
    action('select', t('next.selection.select'), 'i-check'),
    ...(!compact ? [{id: 'file-separator', separator: true as const}] : []),
    action('file-info', t('next.overlays.fileInfo'), 'i-info'),
    {id: 'owner', section: true, ownerOnly: true, label: compact ? <TouchMenuOwner mark={<ServerMark size={24} />}>{t('next.overlays.owner')}</TouchMenuOwner> : t('next.overlays.owner')},
    action('edit', t('next.overlays.editMetadata'), 'i-edit'),
    action('refresh', t('next.overlays.refreshMetadata'), 'i-refresh'),
    action('delete', t('next.overlays.delete'), 'i-trash', true),
  ];
  return <div data-component-only="touch-menu" data-navigations={navigations} data-menus={menus} data-toggles={toggles} style={{height: '100dvh', background: 'var(--color-ink)'}}>
    <div style={{position: 'absolute', left: compact ? 20 : 280, top: compact ? 20 : 360, width: 148}}><PosterCard primaryRef={anchor} model={{title: reference.title, caption: []}}
      fallback={<img alt="" src={`${referenceOrigin}/assets/pool/movies/${reference.art}`} style={{width: '100%', height: '100%', objectFit: 'cover'}} />}
      onNavigate={() => setNavigations(value => value + 1)} moreExpanded={open}
      onMore={element => {anchor.current = element; setMenus(value => value + 1); setOpen(true);}}
      selection={{active: selecting, checked: picked, onToggle: () => {setPicked(value => !value); setToggles(value => value + 1);}, onExit: () => setPicked(false)}} /></div>
    <Menu open={open} onOpenChange={setOpen} anchorRef={anchor} touchContext items={items} title={
      <TouchMenuTitle title={reference.title} compact={compact} kind={t('search.kind.movie')} year={defaultI18n.number(reference.year, {useGrouping: false})}
        artwork={<img alt="" src={`${referenceOrigin}/assets/pool/movies/${reference.art}`} style={{width: '100%', height: '100%', objectFit: 'cover'}} />} />
      } />
  </div>;
}
createRoot(document.getElementById('root')!).render(<><IconSprite /><TouchMenuReference /></>);
