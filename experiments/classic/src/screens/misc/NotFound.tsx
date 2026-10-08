import React from 'react';
import {Screen} from '../../ui/Page';
import {EmptyState} from '../../ui/Controls';
import {root} from '../../nav/nav';

/** A link that doesn't lead anywhere any more. */
export function NotFoundScreen() {
  return (
    <Screen>
      <EmptyState icon="alert" title="That page isn’t here" body="The link may be out of date, or the item may have been removed." action={{label: 'Go to Home', onPress: () => root('Home')}} secondary={{label: 'Search', onPress: () => root('Search')}} />
    </Screen>
  );
}
