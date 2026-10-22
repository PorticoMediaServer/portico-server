/** Native shell labels; shared navigation, Home and action copy stays in its canonical catalogue. */
export const homeNativeMessages = {
  'next.home.native.mainNavigation': 'Main navigation',
  'next.home.native.profileAndServer': 'Profile and server',
  'next.home.native.libraryActions': 'Library actions',
  'next.home.native.switchLibrary': 'Switch library',
  'next.home.native.holdOK': 'Hold OK',
  'next.home.native.moreOptions': 'More options',
} as const satisfies Record<`next.home.native.${string}`, string>;
