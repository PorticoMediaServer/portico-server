import React from 'react';
import {Link} from '@tanstack/react-router';
import {genreFilter} from '@core/presentation/index.ts';
import {encodePredicates, type Predicate} from '../../app/browse';
import s from './Detail.module.css';

/**
 * A title's genres as links: each opens the title's own library on its grid, filtered to that
 * genre (Justin, 2 Oct 2026). The same names the Genre filter lists, so the link always lands on
 * a filter the toolbar shows as applied.
 */
export function GenreLinks({libraryId, genres}: {libraryId: string; genres: readonly string[]}) {
  return (
    <span>
      {genres.map((genre, i) => (
        <React.Fragment key={genre}>
          {i ? ', ' : null}
          <Link className={s.genreLink} to="/library/$libraryId" params={{libraryId}} search={{view: 'browse', filters: encodePredicates([genreFilter(genre) as Predicate])}}>{genre}</Link>
        </React.Fragment>
      ))}
    </span>
  );
}
