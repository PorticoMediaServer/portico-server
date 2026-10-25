import {routePath} from '../../../../packages/contracts/src/routes.ts';
import {parseHomeDestinationPicker} from '@core/home-selection/destination-picker.ts';
import {DetailService} from '@core/detail.ts';
import {HomeSelectionDestinations} from '@core/home-selection/destinations.ts';
import type {HomeSelectionPicker} from '@core/home-selection/types.ts';
import type {SelectionItem, SelectedItem, SelectionTarget} from '@core/selection-model/index.ts';
import type {EntryActionPorts, EntryDestination} from '../../shell/entry-actions';

/** A private checked destination session, never Saved's visible route or store. */
export class LibraryPickerProducer {
  private readonly lifetime = new AbortController();
  private readonly detail: DetailService;
  private session?: HomeSelectionDestinations;
  private flight?: Promise<void>;
  private state?: HomeSelectionPicker;
  constructor(private readonly targets: readonly SelectionItem[], private readonly kind: EntryDestination,
    private readonly ports: EntryActionPorts, private readonly canCreate: boolean, private readonly publish: (state: HomeSelectionPicker) => void) {
    const api = {request: async <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> => {
      this.check();
      const value = await ports.api.request<T>(path, method, body, signal ? AbortSignal.any([signal, this.lifetime.signal]) : this.lifetime.signal);
      this.check(); return value;
    }};
    this.api = api;
    this.detail = new DetailService({api, scope: {serverId: ports.serverId, viewerId: ports.viewerId}, requestId: ports.requestId});
  }
  private readonly api: EntryActionPorts['api'];
  private current = () => !this.lifetime.signal.aborted && this.ports.isCurrent();
  private check() {if (!this.current()) throw new DOMException('Picker authority changed', 'AbortError');}
  private update = (state: HomeSelectionPicker) => {if (this.current()) {this.state = state; this.publish(state);}};
  private run(work: () => Promise<void>): Promise<void> {
    this.check();
    if (this.flight) return this.flight;
    this.flight = Promise.resolve().then(async () => {try {this.check(); await work(); this.check();} finally {this.flight = undefined;}});
    return this.flight;
  }
  open() {return this.run(async () => {
    const identity = this.ports.pickerIdentity;
    if (!identity) throw new Error('picker-identity-required');
    if (!this.targets.length || this.targets.length > 256 || new Set(this.targets.map(item => item.id)).size !== this.targets.length) throw new Error('picker-target-required');
    const picks: SelectedItem[] = [];
    let parents = false;
    let captured: {fence: string; catalog: number; viewer: number} | undefined;
    for (const selected of this.targets) {
      if (!('libraryId' in selected) || selected.capabilities[this.kind] !== true) throw new Error('picker-action-not-permitted');
      if (['show', 'season', 'album', 'artist', 'book', 'collection'].includes(selected.kind)) {
        parents = true;
        picks.push(Object.freeze({key: JSON.stringify([selected.libraryId, selected.kind, selected.id]), item: selected}));
        continue;
      }
      await this.detail.select({libraryId: selected.libraryId, itemId: selected.id}); this.check();
      const data = this.detail.getSnapshot().data;
      if (this.detail.getSnapshot().phase !== 'ready' || !data || data.item.id !== selected.id || data.scope.libraryId !== selected.libraryId
        || data.item.kind !== selected.kind || !['movie', 'episode', 'song', 'audiobook_file'].includes(data.item.kind)) throw new Error('picker-leaf-required');
      const playlist = data.actions.some(action => action.id === 'add_to_playlist' && action.enabled);
      const collection = data.actions.some(action => action.id === 'add_to_collection' && action.enabled);
      if (!(this.kind === 'playlist' ? playlist : collection)) throw new Error('picker-action-not-permitted');
      if (!captured) captured = {fence: data.scope.viewerFence, catalog: data.revision.catalog, viewer: data.revision.viewer};
      if (captured.fence !== data.scope.viewerFence || captured.catalog !== data.revision.catalog || captured.viewer !== data.revision.viewer) throw new Error('picker-selection-changed');
      const item = Object.freeze({...selected, capabilities: Object.freeze({...selected.capabilities, playlist, collection})});
      picks.push(Object.freeze({key: JSON.stringify([item.libraryId, item.kind, item.id]), item}));
    }
    if (parents) {
      // This authenticated read issues the destination domain's own fence. Do
      // not substitute the Search token, a metadata fence or a descendant read.
      const request = {kind: this.kind, query: '', limit: 40};
      const raw = await this.api.request<unknown>(routePath('post_home_destination_picker'), 'POST', request, this.lifetime.signal); this.check();
      if (!raw || typeof raw !== 'object' || !('viewerFence' in raw) || typeof raw.viewerFence !== 'string' || !/^[a-f0-9]{64}$/.test(raw.viewerFence)) throw new Error('picker-authority-required');
      const page = parseHomeDestinationPicker(raw, request, this.ports.serverId, raw.viewerFence);
      captured = {fence: raw.viewerFence, catalog: page.revision.catalog, viewer: page.revision.viewer};
    }
    if (!captured) throw new Error('picker-target-required');
    const viewId = await this.ports.requestId(); this.check();
    const target: SelectionTarget = Object.freeze({mode: 'explicit', items: Object.freeze(picks), displayItems: Object.freeze(picks), context: {
      scope: {serverId: this.ports.serverId, authority: identity.authority, accountId: identity.accountId, profileId: identity.profileId,
        viewId, privacyFence: captured.fence},
      revision: {catalogRevision: String(captured.catalog), viewerRevision: String(captured.viewer)}}});
    this.session = new HomeSelectionDestinations({api: this.api, events: identity.events, requestId: this.ports.requestId}, target, this.kind,
      this.lifetime.signal, this.current, this.update);
    await this.session.load();
  });}
  private required() {this.check(); if (!this.session) throw new Error('picker-not-ready'); return this.session;}
  search(query: string) {return this.run(() => this.required().search(query));}
  page(previous = false) {return this.run(() => this.required().page(previous));}
  choose(id: string) {return this.run(async () => {
    if (!this.state?.rows.some(row => row.id === id && row.canAdd)) throw new Error('destination-not-offered');
    await this.required().choose(id);
  });}
  create(name: string) {return this.run(async () => {
    if (this.kind === 'collection' && !this.canCreate) throw new Error('collection-owner-only');
    await this.required().create(name);
  });}
  retry() {return this.session ? this.run(() => this.required().retry()) : this.open();}
  close() {this.lifetime.abort(); this.session?.dispose(); this.detail.dispose();}
}
