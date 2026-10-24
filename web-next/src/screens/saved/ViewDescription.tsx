import {useEffect, useRef, useState} from 'react';
import type {BrowseCapabilities} from '@core/browse.ts';
import type {PersonalResource} from '@core/personal-saved.ts';
import {savedViewSummary} from '@core/presentation/saved-view-summary.ts';
import {serviceI18n} from '@core/presentation/service-text.ts';
import type {ReadSavedViewCapabilities} from './view-capabilities';

/** The real resource and its library vocabulary are separate reads. No stored
 * free-text summary or previous resource's description stands in while loading. */
export function useSavedViewDescription(resource: PersonalResource | undefined, readCapabilities: ReadSavedViewCapabilities | undefined) {
  const selected = resource?.kind === 'view' && resource.status === 'ready' && resource.definition ? resource : undefined;
  const latest = useRef({selected, readCapabilities});latest.current = {selected, readCapabilities};
  const [ready, setReady] = useState<Readonly<{resource: PersonalResource; readCapabilities: ReadSavedViewCapabilities; capabilities: BrowseCapabilities}>>();
  useEffect(() => {
    if (!selected?.definition || !readCapabilities) return;
    const request = new AbortController();
    void readCapabilities(selected.definition, request.signal).then(capabilities => {
      if (!request.signal.aborted && latest.current.selected === selected && latest.current.readCapabilities === readCapabilities) setReady({resource: selected, readCapabilities, capabilities});
    }, () => { /* Missing vocabulary withholds the line under §38. */ });
    return () => request.abort();
  }, [selected, readCapabilities]);
  return selected?.definition && ready?.resource === selected && ready.readCapabilities === readCapabilities ? savedViewSummary(selected.definition, ready.capabilities, serviceI18n()) : undefined;
}

export function SavedViewDescription({resource, readCapabilities}: {resource: PersonalResource; readCapabilities?: ReadSavedViewCapabilities}) {
  const summary = useSavedViewDescription(resource, readCapabilities);
  return summary ? <small>{summary.line}</small> : null;
}
