import React from 'react';
import {View} from 'react-native';
import {useClient} from '../../data/DataProvider';
import {Group, ListRow} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {useMetrics} from '../../theme/layout';
import {openScreen, SCREENS} from '../../nav/screens';

/**
 * Every screen and state, for review (also reachable as
 * portico-classic://screen/<id>).
 */
export function ScreenIndex() {
  const m = useMetrics();
  const client = useClient();
  const groups = [...new Set(SCREENS.map(s => s.group))];
  return (
    <View style={{gap: m.space.lg}}>
      <Text variant="caption" tone="tertiary">Deep link: portico-classic://screen/&lt;id&gt; — for example portico-classic://screen/movie.</Text>
      {groups.map(g => (
        <Group key={g} title={g}>
          {SCREENS.filter(s => s.group === g).map(s => <ListRow key={s.id} title={s.label} subtitle={s.id} onPress={() => openScreen(s.id, client)} />)}
        </Group>
      ))}
    </View>
  );
}
