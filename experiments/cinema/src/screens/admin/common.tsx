import React from 'react';
import {View} from 'react-native';
import type {AdminState} from '../../data/admin';
import {useQuery, useServices} from '../../data/hooks';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';

/** Reads one console section and returns a patcher that saves it back. */
export function useAdmin<K extends keyof AdminState>(key: K) {
  const s = useServices();
  const q = useQuery('admin:' + key, () => s.admin.get(key));
  const save = (patch: Partial<AdminState[K]> | AdminState[K]) => {
    const next = Array.isArray(q.data) ? (patch as AdminState[K]) : ({...(q.data as object), ...(patch as object)} as AdminState[K]);
    return s.admin.set(key, next);
  };
  return {data: q.data, save, run: s.admin.run, loading: q.loading};
}

export {ago} from '../../design/time';

/** Small bar chart for telemetry. */
export function Bars({values, color = colors.accentStrong, height = 60, label, unit = '%'}: {values: number[]; color?: string; height?: number; label: string; unit?: string}) {
  const max = Math.max(1, ...values);
  const h = u(height);
  return (
    <View style={{flex: 1, minWidth: isTV ? 360 : 150}}>
      <View style={{flexDirection: 'row', justifyContent: 'space-between'}}>
        <Text variant="captionStrong" tone="secondary">{label}</Text>
        <Text variant="captionStrong">{Math.round(values[values.length - 1] ?? 0)}{unit}</Text>
      </View>
      <View style={{flexDirection: 'row', alignItems: 'flex-end', height: h, gap: 2, marginTop: 6}}>
        {values.map((v, i) => <View key={i} style={{flex: 1, height: Math.max(2, (v / max) * h), backgroundColor: color, opacity: 0.35 + 0.65 * (i / values.length), borderRadius: 2}} />)}
      </View>
    </View>
  );
}

export function KV({k, v, tone}: {k: string; v: string; tone?: 'healthy' | 'amber' | 'danger'}) {
  return (
    <View style={{flexDirection: 'row', justifyContent: 'space-between', paddingVertical: u(6), paddingHorizontal: u(14), borderBottomWidth: 1, borderBottomColor: 'rgba(28,51,65,0.6)'}}>
      <Text variant="caption" tone="tertiary">{k}</Text>
      <Text variant="captionStrong" tone={tone ?? 'primary'} style={{maxWidth: '65%', textAlign: 'right'}}>{v}</Text>
    </View>
  );
}
