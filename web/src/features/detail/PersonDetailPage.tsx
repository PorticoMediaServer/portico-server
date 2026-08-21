import { RefreshCw, UserRound } from '#portico-icons';
import { productMessage, type ProductMessagePresentation } from '@portico/client-core';
import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { SecondaryButton } from '../../components/controls/Buttons';
import { ProductLanguageIcon, productLanguageProblem } from '../../components/states/ProductLanguageState';
import { usePersonDetail, usePorticoDataSource } from '../../data/DataProvider';
import type { PersonDetail } from '../../data/models';
import { SectionHeading, SelectableMediaGrid } from '../catalog/CatalogSurface';
import './detail.css';

function initials(name: string) {
  return name.split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]?.toLocaleUpperCase()).join('') || '?';
}

export function PersonDetailPage() {
  const { id } = useParams();
  const [reloadKey, setReloadKey] = useState(0);
  const [page, setPage] = useState<PersonDetail>();
  const [loadingMore, setLoadingMore] = useState(false);
  const [pageError, setPageError] = useState<ProductMessagePresentation>();
  const continuation = useRef<{ controller: AbortController; generation: number } | undefined>(undefined);
  const generation = useRef(0);
  const source = usePorticoDataSource();
  const detail = usePersonDetail(id, reloadKey);
  useEffect(() => {
    if (detail.status === 'success') setPage(detail.data);
  }, [detail]);
  useEffect(() => {
    generation.current += 1;
    continuation.current?.controller.abort();
    setLoadingMore(false);
    setPageError(undefined);
    return () => continuation.current?.controller.abort();
  }, [id]);
  const loadingMessage = productMessage('media.detail-loading');
  const retryLabel = productMessage('action.retry').text;
  const loadingMoreMessage = productMessage('state.loading-more');
  const creditsLabel = productMessage('person.credits-label').text ?? '';
  const moreCreditsLabel = productMessage('action.load-more-group', { group: creditsLabel.toLocaleLowerCase() }).text;
  if (detail.status === 'loading') return <div className="standard-page"><div className="library-state" aria-live="polite" aria-busy="true"><ProductLanguageIcon presentation={loadingMessage} /><strong>{loadingMessage.title}</strong><p>{loadingMessage.body}</p></div></div>;
  if (detail.status === 'error') {
    const presentation = productLanguageProblem(detail.error, 'media.detail-unavailable');
    return <div className="standard-page"><div className="library-state error" role="alert"><ProductLanguageIcon presentation={presentation} /><strong>{presentation.title}</strong><p>{presentation.body}</p><SecondaryButton onClick={() => setReloadKey((value) => value + 1)}><RefreshCw /> {retryLabel}</SecondaryButton></div></div>;
  }

  const person = page ?? detail.data;
  const emptyMessage = productMessage('media.children-empty');
  const loadMore = async () => {
    if (!person.nextCursor || loadingMore) return;
    continuation.current?.controller.abort();
    const controller = new AbortController();
    const requestGeneration = ++generation.current;
    continuation.current = { controller, generation: requestGeneration };
    setLoadingMore(true);
    setPageError(undefined);
    try {
      const next = await source.person(person.id, controller.signal, person.nextCursor);
      if (controller.signal.aborted || generation.current !== requestGeneration) return;
      setPage((current) => {
        const visible = current ?? person;
        if (visible.id !== person.id) return current;
        const seen = new Set(visible.credits.map((credit) => credit.id));
        return { ...next, credits: [...visible.credits, ...next.credits.filter((credit) => !seen.has(credit.id))] };
      });
    } catch (reason) {
      if (!controller.signal.aborted && generation.current === requestGeneration) setPageError(productLanguageProblem(reason));
    } finally {
      if (generation.current === requestGeneration) setLoadingMore(false);
    }
  };
  return <div className="standard-page portico-person-detail-page">
    <nav className="portico-detail-breadcrumbs" aria-label={productMessage('person.breadcrumb-label').text}><Link to="/search">{productMessage('destination.search').text}</Link><span>/</span><strong aria-current="page">{person.name}</strong></nav>
    <header className="portico-person-detail-header">
      <div className="portico-person-detail-portrait">{person.imageUrl ? <img src={person.imageUrl} alt="" /> : <><UserRound aria-hidden="true" /><strong>{initials(person.name)}</strong></>}</div>
      <div><p className="portico-detail-kind">{productMessage('person.kind-label').text}</p><h1>{person.name}</h1>{person.knownFor && <p className="portico-person-known-for">{productMessage('person.known-for', { title: person.knownFor }).text}</p>}{person.biography && <p>{person.biography}</p>}</div>
    </header>
    <section className="portico-detail-section">
      <SectionHeading title={productMessage('person.credits-title').text ?? ''} detail={productMessage('person.credits-count', { count: `${person.credits.length}${person.hasMore ? '+' : ''}` }).text} />
      {person.credits.length > 0 ? <SelectableMediaGrid items={person.credits} className="person-credit-grid" playbackContext={{ type: 'search', id: person.id, title: person.name }} /> : <div className="portico-detail-inline-state empty" role="status"><ProductLanguageIcon presentation={emptyMessage} /><span><strong>{emptyMessage.title}</strong><small>{emptyMessage.body}</small></span></div>}
      {pageError && <div className="portico-detail-inline-state error" role="alert"><ProductLanguageIcon presentation={pageError} /><span><strong>{pageError.title}</strong><small>{pageError.body}</small></span><SecondaryButton onClick={() => void loadMore()}><RefreshCw /> {retryLabel}</SecondaryButton></div>}
      {person.hasMore && person.nextCursor && <SecondaryButton disabled={loadingMore} onClick={() => void loadMore()}>{loadingMore ? <RefreshCw className="state-spinner" /> : null} {loadingMore ? loadingMoreMessage.title : moreCreditsLabel}</SecondaryButton>}
    </section>
  </div>;
}
