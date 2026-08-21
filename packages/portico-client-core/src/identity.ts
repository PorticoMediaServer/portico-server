export const PORTICO_USERNAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{1,30}[A-Za-z0-9]$/;

export function normalizePorticoUsername(value: string): string {
  return value.trim().toLocaleLowerCase("en-US");
}

export function validPorticoUsername(value: string): boolean {
  return PORTICO_USERNAME_PATTERN.test(value.trim());
}
