export const cx = (...parts: unknown[]): string => parts.filter(p => typeof p === 'string' && p).join(' ');
