export const platformMessages = {
  'next.platform.appName': 'Portico',
  'next.platform.galleryTitle': 'Kit',
  'next.platform.kitTitle': 'Component kit',
  'next.platform.allFamilies': 'All components',
  'next.platform.galleryFamily': 'Component family',
  'next.platform.galleryCase': 'State',
} as const satisfies Record<`next.platform.${string}`, string>;
