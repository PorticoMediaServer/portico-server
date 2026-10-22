/** Native shell gallery and platform-specific wording. Shared product wording lives in shell.ts. */
export const shellNativeMessages = {
  'next.shell.native.galleryTitle': 'Shell',
  'next.shell.native.identityGallery': 'Server identity',
  'next.shell.native.accountGallery': 'Profiles and account',
  'next.shell.native.settingsGallery': 'Settings',
  'next.shell.native.peopleGallery': 'People and devices',
  'next.shell.native.statisticsGallery': 'Statistics',
  'next.shell.native.segmentsGallery': 'Intros and credits',
} as const satisfies Record<`next.shell.native.${string}`, string>;
