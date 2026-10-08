import {errorText} from './errors';
import {useI18n} from './i18n';
import React, {createContext, useCallback, useContext, useEffect, useMemo, useRef, useState} from 'react';
import {PreferencesReader} from '@core/preferences-reader.ts';
import {preferenceValue, type PreferencePatch, type PreferenceScope, type PreferenceSnapshot, type PreferenceValue} from '@core/preferences.ts';
import {useSession} from './session';
import {useViewerScope} from './viewer-scope';

/**
 * Viewer preferences published by the selected server. The server owns every
 * default, domain and clamp; this provider only mirrors the snapshot and
 * forwards merge patches. When the server cannot be reached the snapshot is
 * absent and readers fall back to the defaults they pass in, so the shell
 * keeps working without it.
 */
type Value = {
  snapshot?: PreferenceSnapshot;
  loading: boolean;
  error?: string;
  saving: boolean;
  /** Effective value for a registry key, or the fallback when the server has not answered. */
  value: <T extends PreferenceValue>(key: string, fallback: T) => T;
  /** Merge a patch into the scope each field declares. Keys are grouped by scope automatically. */
  set: (patch: PreferencePatch) => Promise<void>;
  reload: () => void;
};

const Ctx = createContext<Value>({loading: false, saving: false, value: (_k, f) => f, set: async () => {}, reload: () => {}});

export function ServerPreferencesProvider({children}: {children: React.ReactNode}) {
  const {api} = useSession();
  const scope = useViewerScope();
  const client = useMemo(() => new PreferencesReader(api, scope), [api, scope]);
  useEffect(() => () => client.dispose(), [client]);
  const [snapshot, setSnapshot] = useState<PreferenceSnapshot>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const generation = useRef(0);
  const load = useCallback(() => {
    const gen = ++generation.current;
    setLoading(true);
    client.preferences('web').then(next => {
      if (gen !== generation.current) return;
      setSnapshot(next);
      setError(undefined);
    }).catch((e: unknown) => {
      if (gen !== generation.current) return;
      setError(errorText(e, 'preferences', 'load'));
    }).finally(() => {
      if (gen === generation.current) setLoading(false);
    });
  }, [client]);
  useEffect(load, [load]);
  const value = useCallback(<T extends PreferenceValue>(key: string, fallback: T): T => {
    if (!snapshot) return fallback;
    try {
      return preferenceValue<T>(snapshot, key);
    } catch {
      return fallback;
    }
  }, [snapshot]);
  const set = useCallback(async (patch: PreferencePatch) => {
    if (!snapshot) throw new Error('Preferences are not available while the server is unreachable.');
    const byScope = new Map<PreferenceScope, Record<string, PreferenceValue | null>>();
    for (const [key, v] of Object.entries(patch)) {
      const field = snapshot.registry.fields.find(f => f.key === key);
      if (!field) throw new Error(`This server does not publish the preference ${key}.`);
      const scopeName = field.scopes[0];
      if (!byScope.has(scopeName)) byScope.set(scopeName, {});
      byScope.get(scopeName)![key] = v;
    }
    setSaving(true);
    try {
      let current = snapshot;
      for (const [scopeName, values] of byScope) {
        const revision = current.documents.find(d => d.scope === scopeName)?.revision ?? 1;
        current = await client.applyPreferences(scopeName, 'web', revision, values);
      }
      setSnapshot(current);
      setError(undefined);
    } finally {
      setSaving(false);
    }
  }, [client, snapshot]);
  const ctx = useMemo<Value>(() => ({snapshot, loading, error, saving, value, set, reload: load}), [snapshot, loading, error, saving, value, set, load]);
  return <Ctx.Provider value={ctx}><RegionSync />{children}</Ctx.Provider>;
}

export const useServerPreferences = () => useContext(Ctx);

/** X-03: keeps the active catalogue (errors, UI primitives, code outside render) on the viewer's region. */
function RegionSync() {
  useI18n();
  return null;
}
