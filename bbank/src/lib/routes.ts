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
