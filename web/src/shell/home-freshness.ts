import {dependsOn, type FreshnessHub} from '@core/freshness-hub.ts';
import type {HomeService} from '@core/home-service.ts';

export const homeDependencies = [dependsOn.libraries, dependsOn.content(), dependsOn.order(), dependsOn.personal(), dependsOn.progress(),
  dependsOn.settings('preferences'), dependsOn.settings('server'), dependsOn.list('recommendations'), dependsOn.list('guide'),
  dependsOn.list('liveSources'), dependsOn.list('libraryChannels'), dependsOn.clock('home-time')];

export class HomeReadNotAnswered extends Error {
  readonly retryable = true;
  constructor() {super('Home read was not adopted.'); this.name = 'HomeReadNotAnswered';}
}

/** Shell owns this display registration and the service; the route only opens/closes it. */
export function createShellHomeDocument(hub: FreshnessHub, service: HomeService) {
  const document = hub.document({dependencies: homeDependencies,
    hasContent: () => !!service.getSnapshot().document,
    revalidate: async context => {
      const result = await service.refresh(context);
      if (result !== 'adopted') throw new HomeReadNotAnswered();
    },
    recover: async context => {
      if (context.signal.aborted) throw new HomeReadNotAnswered();
      await service.load();
      const snapshot = service.getSnapshot();
      if (!snapshot.document || snapshot.phase !== 'ready') throw snapshot.error ?? new HomeReadNotAnswered();
    }});
  const off = service.subscribe(() => document.check());
  return {document, dispose: () => {off(); document.dispose();}};
}
