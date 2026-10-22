/** Native shell labels; shared navigation, Home and action copy stays in its canonical catalogue. */
export const homeNativeMessages = {
  'next.home.native.mainNavigation': 'Main navigation',
  'next.home.native.profileAndServer': 'Profile and server',
  'next.home.native.selectRow': 'Select row',
} as const satisfies Record<`next.home.native.${string}`, string>;
