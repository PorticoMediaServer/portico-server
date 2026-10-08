import React from 'react';
import {useRoute} from '@react-navigation/native';
import {Screen} from '../../nav/Screen';
import {Text} from '../../design/Text';

/** Temporary stand-in while a screen is being built. */
export function Placeholder() {
  const r = useRoute();
  return (
    <Screen heading={r.name}>
      <Text tone="secondary">{JSON.stringify(r.params ?? {})}</Text>
    </Screen>
  );
}
