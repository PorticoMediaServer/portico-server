import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {BrowseFacetPage, BrowseFacetValue} from '@portico/client-core';
import {Button} from '../../kit/controls';
import {Skeleton, StateView} from '../../kit/states';
import {loadFacetPage, type FacetRequest, type ReadingFacetField} from './facets';
import styles from './facet-page.module.css';

const {t} = defaultI18n;

/**
 * phone-read 1-3 (Root §2): the non-browsable Authors and Series surfaces list counted accepted
 * values, never entity headers. Values come from the server's counted facet; a page never invents
 * a value, a count or a series identity, and a changed or unready facet is reported rather than
 * drawn as an empty library.
 */
export function FacetPage({libraryId, field, pivot, request, onSelect}: {
  libraryId: string; field: ReadingFacetField; pivot: string; request: FacetRequest;
  /** Absent when the surface has no working destination yet (Series until its ordered view). */
  onSelect?: (value: BrowseFacetValue) => void;
}) {
  const [page, setPage] = useState<BrowseFacetPage>(), [values, setValues] = useState<readonly BrowseFacetValue[]>([]);
  const [failed, setFailed] = useState(false), [more, setMore] = useState(false), [retry, setRetry] = useState(0);
  const lifetime = useRef<AbortController>(undefined);
  const pending = useRef(false), offset = useRef(0);
  useEffect(() => {
    const operation = new AbortController();
    lifetime.current = operation; pending.current = false; offset.current = 0;
    setFailed(false); setPage(undefined); setValues([]); setMore(false);
    void loadFacetPage(request, {libraryId, field, pivot}, operation.signal)
      .then(first => {if (!operation.signal.aborted) {offset.current = first.values.length; setPage(first); setValues(first.values);}})
      .catch(() => {if (!operation.signal.aborted) setFailed(true);});
    return () => operation.abort();
  }, [libraryId, field, pivot, request, retry]);
  const loadMore = () => {
    const operation = lifetime.current;
    if (!page || pending.current || !operation || operation.signal.aborted) return;
    pending.current = true;
    setMore(true);
    void loadFacetPage(request, {libraryId, field, pivot, offset: offset.current}, operation.signal)
      .then(next => {if (!operation.signal.aborted) {
        if (!next.values.length && offset.current < next.total) throw new Error('invalid_document');
        offset.current += next.values.length; setPage(next);
        setValues(current => [...current, ...next.values.filter(entry => !current.some(seen => seen.value === entry.value))]);
      }})
      .catch(() => {if (!operation.signal.aborted) setFailed(true);})
      .finally(() => {if (!operation.signal.aborted) {pending.current = false; setMore(false);}});
  };
  if (failed || page && values.length === 0) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setRetry(value => value + 1)}}/>;
  if (!page) return <Skeleton height={320}/>;
  return <div className={styles.facet}>
    {values.map(entry => onSelect ? <button key={entry.value} type="button" className={styles.value} onClick={() => onSelect(entry)}>
      <span className={styles.label}>{entry.label}</span>
      <span className={styles.count}>{defaultI18n.number(entry.count)}{entry.countIsLowerBound ? '+' : ''}</span>
    </button> : <div key={entry.value} className={`${styles.value} ${styles.static}`}>
      <span className={styles.label}>{entry.label}</span>
      <span className={styles.count}>{defaultI18n.number(entry.count)}{entry.countIsLowerBound ? '+' : ''}</span>
    </div>)}
    {offset.current < page.total && <div className={styles.more}><Button variant="glass" onClick={loadMore} disabled={more}>{t('action.showMore')}</Button></div>}
  </div>;
}
