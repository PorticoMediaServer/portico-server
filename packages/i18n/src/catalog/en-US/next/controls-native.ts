/** Native controls copy; existing product actions reuse the shared catalogue. */
export const controlsNativeMessages = {
  'next.controls.native.choose': 'Choose',
  'next.controls.native.galleryTitle': 'Controls',
  'next.controls.native.buttonsTitle': 'Buttons',
  'next.controls.native.resumeTitle': 'Resume',
  'next.controls.native.chipsTitle': 'Filters',
  'next.controls.native.tabsTitle': 'Tabs',
  'next.controls.native.segmentedTitle': 'Choices',
  'next.controls.native.switchesTitle': 'Switches',
  'next.controls.native.fieldsTitle': 'Search',
  'next.controls.native.selectTitle': 'Playback',
  'next.controls.native.richSelectTitle': 'Quality & data',
} as const satisfies Record<`next.controls.native.${string}`, string>;
