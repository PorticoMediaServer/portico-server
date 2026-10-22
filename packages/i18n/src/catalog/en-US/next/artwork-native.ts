export const artworkNativeMessages = {
  'next.artwork.native.galleryTitle': 'When the artwork isn’t there',
  'next.artwork.native.backdrop': 'Backdrop',
  'next.artwork.native.posterOnly': 'Poster only',
  'next.artwork.native.noArtwork': 'No artwork',
  'next.artwork.native.manageProfiles': 'Manage profiles',
  'next.artwork.native.timeMachine': 'The Time Machine',
  'next.artwork.native.wells': 'H. G. Wells',
  'next.artwork.native.dracula': 'Dracula',
  'next.artwork.native.stoker': 'Bram Stoker',
  'next.artwork.native.metamorphosis': 'The Metamorphosis',
  'next.artwork.native.kafka': 'Franz Kafka',
  'next.artwork.native.warOfWorlds': 'The War of the Worlds',
} as const satisfies Record<`next.artwork.native.${string}`, string>;
