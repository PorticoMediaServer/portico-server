/** Native component-gallery names; product actions reuse the shared catalogue. */
export const controlsNativeMessages = {
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

/** Sample facts imported only by the native development gallery, never registered. */
export const nativeControlsFixtureMessages = {
  'next.controls.native.fixture.longTitle': 'The Lord of the Rings: The Fellowship of the Ring',
} as const satisfies Record<`next.controls.native.fixture.${string}`, string>;
