import 'server-only'
import { cookies } from 'next/headers'
import { verifyAccessToken, type Claims, type Role } from './jwt'
import { flash } from './flash'
import { COOKIE_SECURE } from './cookies'

export const ACCESS_COOKIE = 'bb_at'
export const REFRESH_COOKIE = 'bb_rt'

// The refresh cookie is deliberately scoped to the one path that consumes it
// (TRD §7.3), so ordinary navigation never carries it. `src/app/api/v1/auth/
// refresh/route.ts` exists on this origin at that exact path for that reason.
export const REFRESH_PATH = '/api/v1/auth/refresh'

export type Session = Claims

/**
 * The signed-in user, or null.
 *
 * This used to be `JSON.parse` of a cookie the browser could edit by hand. It is
 * now an ES256 signature check: changing one byte of the cookie makes the
 * session vanish rather than making the user an admin.
 */
export async function getSession(): Promise<Session | null> {
    const store = await cookies()
    return verifyAccessToken(store.get(ACCESS_COOKIE)?.value)
}

/** The session, or a redirect to the login page. For pages that require one. */
export async function requireSession(): Promise<Session> {
    const session = await getSession()
    if (!session) {
        const { redirect } = await import('next/navigation')
        // redirect() throws; the return keeps the compiler from widening the
        // type to `Session | null` for every caller.
        return redirect(flash('/login', { error: 'Please log in' }))
    }
    return session
}

export async function hasRole(...roles: Role[]): Promise<boolean> {
    const s = await getSession()
    return !!s && roles.includes(s.role)
}

/**
 * Copies the API's Set-Cookie headers onto this origin's response.
 *
 * The browser talks to Next.js and Next.js talks to the Go API, so the API's
 * cookies land on a server-to-server response the browser never sees. Without
 * this the login would succeed and the user would still be signed out.
 *
 * Only the two known auth cookies are copied. Blindly forwarding whatever the
 * upstream sets would let a compromised API set arbitrary cookies on this origin.
 *
 * Callable only from a server action or a route handler — Next.js does not allow
 * a component render to set cookies.
 */
export async function adoptAuthCookies(apiResponse: Response): Promise<void> {
    const store = await cookies()

    for (const header of apiResponse.headers.getSetCookie()) {
        const [pair] = header.split(';')
        const idx = pair.indexOf('=')
        if (idx < 0) continue
        const name = pair.slice(0, idx).trim()
        const value = pair.slice(idx + 1).trim()

        if (name === ACCESS_COOKIE) {
            store.set(ACCESS_COOKIE, value, {
                httpOnly: true, sameSite: 'lax', secure: COOKIE_SECURE, path: '/',
            })
        } else if (name === REFRESH_COOKIE) {
            store.set(REFRESH_COOKIE, value, {
                // Strict, and confined to the refresh path: it is never sent on
                // ordinary navigation, only to the endpoint that consumes it.
                httpOnly: true, sameSite: 'strict', secure: COOKIE_SECURE, path: REFRESH_PATH,
            })
        }
    }
}

// There is deliberately no `clearSession()` helper. Deleting the cookies is the
// easy half of logging out and the half that does not matter: it leaves the
// refresh-token family valid server-side for its full life. Sign-out goes
// through `app/api/v1/auth/refresh/logout/route.ts`, which is reachable by the
// refresh cookie and so can revoke it first.
