/** Same creation/change policy as Hosted; old password hashes remain valid. */
export function validDirectPassword(value:string):boolean {
 return Array.from(value).length>=8&&new TextEncoder().encode(value).length<=72&&/\p{Lu}/u.test(value)&&/\p{Ll}/u.test(value)&&/[^\p{L}]/u.test(value);
}
