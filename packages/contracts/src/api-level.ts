/** The API generation this client speaks. A server answers its own in `GET /v1/system`
 * (`apiLevel`); before release a client opens a server only when the two are equal.
 * The server's `APILevel` (server/internal/httpapi/system_info.go) carries the same number. */
export const API_LEVEL = 1;
