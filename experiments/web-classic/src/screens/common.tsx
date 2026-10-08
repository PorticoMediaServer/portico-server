import React from 'react';
import {useParams, useSearch} from '@tanstack/react-router';
import {Skeleton} from '@/ui/Bits';
import {CardSkeleton} from '@/ui/Card';
import s from './common.module.css';

/** Route params and search without strict typing (the router is not type-registered). */
export const useRouteParams = <T extends Record<string, string>>() => useParams({strict: false}) as unknown as T;
export const useRouteSearch = <T extends Record<string, string | undefined>>() => useSearch({strict: false}) as unknown as T;

/** Generic page skeleton while a route's code or data loads. */
export function PageLoading() {
  return (
    <div className={s.loading} aria-busy="true" aria-label="Loading">
      <Skeleton width={260} height={30} />
      <Skeleton width={420} height={14} style={{marginTop: 12}} />
      <div className={s.cards}>{Array.from({length: 7}, (_, i) => <CardSkeleton key={i} />)}</div>
      <div className={s.cards}>{Array.from({length: 7}, (_, i) => <CardSkeleton key={i} />)}</div>
    </div>
  );
}

/** Hero-shaped skeleton for detail pages. */
export function DetailLoading() {
  return (
    <div className={s.detail} aria-busy="true" aria-label="Loading">
      <div className={s.detailInner}>
        <Skeleton className={s.detailPoster} />
        <div style={{flex: 1, minWidth: 0}}>
          <Skeleton width="55%" height={38} />
          <Skeleton width="35%" height={14} style={{marginTop: 16}} />
          <Skeleton width="90%" height={12} style={{marginTop: 26}} />
          <Skeleton width="84%" height={12} style={{marginTop: 10}} />
          <Skeleton width="60%" height={12} style={{marginTop: 10}} />
          <div style={{display: 'flex', gap: 10, marginTop: 28}}><Skeleton width={120} height={44} /><Skeleton width={44} height={44} /><Skeleton width={44} height={44} /></div>
        </div>
      </div>
    </div>
  );
}

export function Meta({items}: {items: (React.ReactNode | false | null | undefined)[]}) {
  const list = items.filter(Boolean);
  return <span className={s.meta}>{list.map((x, i) => <React.Fragment key={i}>{i ? <span className={s.dot} aria-hidden>·</span> : null}<span>{x}</span></React.Fragment>)}</span>;
}
