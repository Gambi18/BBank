/**
 * Shared route constants.
 *
 * Deliberately a plain module: no `'use server'` (every export there must be an
 * async function, and these are strings) and no `'server-only'` (client
 * components need them too).
 */

/**
 * Where the sign-out forms post.
 *
 * Logging out is NOT a server action, and the reason is subtle enough to be
 * worth stating here: a server action posts to the page's own URL, and `bb_rt`
 * is scoped to `Path=/api/v1/auth/refresh`, so the refresh token is never sent
 * to it. Such an action could only delete cookies — it could not revoke the
 * token family, which is the part that matters.
 * `app/api/v1/auth/refresh/logout/route.ts` sits inside the cookie's path and
 * does both.
 */
export const LOGOUT_ACTION = '/api/v1/auth/refresh/logout'

/**
 * Carries a freshly minted invitation token from the action that created it to
 * the page that displays it, exactly once.
 *
 * A cookie rather than a query parameter: a query string lands in the address
 * bar, in browser history, in the reverse-proxy access log and in the `Referer`
 * of anything the page subsequently loads — four places a one-time credential
 * outlives the moment it was needed, which defeats storing only its hash.
 *
 * HttpOnly, `SameSite=Strict`, and scoped by path to the one page that reads
 * it. It expires after five minutes rather than being deleted on read: Next.js
 * forbids mutating cookies during a render, so the page that displays the token
 * cannot clear it. The short lifetime is the substitute — a reload inside that
 * window shows the link again, which is survivable because the cookie never
 * leaves the browser and is unreadable to script.
 *
 * `WI-79` removes it by emailing the link instead.
 */
export const INVITE_TOKEN_COOKIE = 'bb_invite_once'
