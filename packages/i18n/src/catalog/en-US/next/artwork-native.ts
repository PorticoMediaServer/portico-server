export const artworkNativeMessages = {
  'next.artwork.native.galleryTitle': 'When the artwork isn’t there',
  'next.artwork.native.backdrop': 'Backdrop',
  'next.artwork.native.posterOnly': 'Poster only',
  'next.artwork.native.noArtwork': 'No artwork',
  'next.artwork.native.manageProfiles': 'Manage profiles',
} as const satisfies Record<`next.artwork.native.${string}`, string>;
