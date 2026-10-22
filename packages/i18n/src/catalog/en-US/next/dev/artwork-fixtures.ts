/** Private reference-domain words. Imported only by the serve-only artwork family. */
export const artworkFixtures = {
  'next.artwork.fixture.server': 'On Portico demo',
  'next.artwork.fixture.showArtwork': 'Show artwork',
  'next.artwork.fixture.timeMachine': 'The Time Machine',
  'next.artwork.fixture.wells': 'H. G. Wells',
  'next.artwork.fixture.war': 'The War of the Worlds',
  'next.artwork.fixture.demo': 'Demo',
  'next.artwork.fixture.alex': 'Alex',
  'next.artwork.fixture.kids': 'Kids',
  'next.artwork.fixture.sam': 'Sam',
  'next.artwork.fixture.jules': 'Jules',
} as const satisfies Record<`next.artwork.fixture.${string}`, string>;
