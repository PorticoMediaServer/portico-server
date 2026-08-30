import { StatusWarningIcon, NavigationLibraryIcon, ActionRefreshIcon } from '#portico-icons';
import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { SecondaryButton } from '../components/controls/Buttons';
import { productProblemText, productText } from '../components/ProductLanguage';
import { useLibraries, usePorticoDataSource } from '../data/DataProvider';
import { LibraryWorkspacePage } from '../features/library/LibraryWorkspacePage';
import type { LibraryWorkspaceSource } from '../features/library/libraryTypes';

function isLibraryWorkspaceSource(value: unknown): value is LibraryWorkspaceSource {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<LibraryWorkspaceSource>;
  return typeof candidate.libraryBrowseCapabilities === 'function'
    && typeof candidate.libraryPivot === 'function'
    && typeof candidate.createSavedView === 'function';
}

export function LibraryWorkspaceRoute() {
  const { libraryId = '' } = useParams();
  const source = usePorticoDataSource();
  const [reloadKey, setReloadKey] = useState(0);
  const libraries = useLibraries(reloadKey);

  if (libraries.status === 'loading') {
    return <div className="standard-page library-route-reservation" aria-busy="true" />;
  }
  if (libraries.status === 'error') {
    return <div className="standard-page"><div className="library-state error" role="alert"><StatusWarningIcon /><strong>Couldn’t open this library</strong><p>{productProblemText(libraries.error, 'library.load-failed')}</p><SecondaryButton onClick={() => setReloadKey((value) => value + 1)}><ActionRefreshIcon /> {productText('action.retry')}</SecondaryButton></div></div>;
  }
  const library = libraries.data.find((candidate) => candidate.id === libraryId);
  if (!library) {
    return <div className="standard-page"><div className="library-state error"><NavigationLibraryIcon /><strong>This library isn’t available</strong><p>It may have been removed or is no longer shared with this account.</p><Link className="button secondary" to="/libraries">Open libraries</Link></div></div>;
  }
  if (!isLibraryWorkspaceSource(source)) {
    return <div className="standard-page"><div className="library-state error"><StatusWarningIcon /><strong>This client can’t browse the library</strong><p>Reconnect to a compatible Portico server and try again.</p></div></div>;
  }
  return <LibraryWorkspacePage library={library} source={source} />;
}
