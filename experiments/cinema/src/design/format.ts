/** "1 title", "3 titles". `many` defaults to `one` + "s". */
export function plural(n: number | string, one: string, many = one + 's') {
  return `${n} ${Number(n) === 1 ? one : many}`;
}
