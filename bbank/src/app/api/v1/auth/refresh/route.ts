import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { API_BASE } from '@/lib/api'
import { REFRESH_COOKIE, ACCESS_COOKIE, REFRESH_PATH } from '@/lib/session'
import { flash } from '@/lib/flash'
import { COOKIE_SECURE } from '@/lib/cookies'

/**
 * Sits at exactly the path the refresh cookie is scoped to.
 *
 * `bb_rt` is `SameSite=Strict; Path=/api/v1/auth/refresh` (TRD §7.3) so that it
 * is never sent on ordinary navigation. That property is worth keeping, but it
 * also means nothing else on this origin can read it — so the refresh has to
 * happen *here*, at that path, and the proxy redirects into it when it finds an
 * expired access token.
 *
 * Rotation is the reason this is a route handler and not something the API
 * client does inline: each refresh invalidates the previous token, so a refresh
 * whose new cookie is not persisted would leave the browser holding a token the
 * server has already retired — and presenting it again trips reuse detection and
 * revokes the whole family. Only a route handler or a server action can set
 * cookies, so only they may refresh.
 */
export async function GET(req: NextRequest) {
    const next = safeNext(req.nextUrl.searchParams.get('next'))
    const refresh = req.cookies.get(REFRESH_COOKIE)?.value

    const fail = () => {
        const res = NextResponse.redirect(new URL(flash('/login', { error: 'Your session expired' }), req.url))
        res.cookies.delete(ACCESS_COOKIE)
        res.cookies.delete({ name: REFRESH_COOKIE, path: REFRESH_PATH })
        return res
    }

    if (!refresh) return fail()

    let upstream: Response
    try {
        upstream = await fetch(`${API_BASE}${REFRESH_PATH}`, {
            method: 'POST',
            headers: { Cookie: `${REFRESH_COOKIE}=${refresh}` },
            cache: 'no-store',
        })
    } catch {
        return fail()
    }
    if (!upstream.ok) return fail()

    // `_r` tells the proxy this navigation has already been through a refresh,
    // so a token that still will not verify ends at the login page instead of
    // bouncing back here forever.
    const to = new URL(next, req.url)
    to.searchParams.set('_r', '1')
    const res = NextResponse.redirect(to)

    for (const header of upstream.headers.getSetCookie()) {
        const [pair] = header.split(';')
        const idx = pair.indexOf('=')
        if (idx < 0) continue
        const name = pair.slice(0, idx).trim()
        const value = pair.slice(idx + 1).trim()
        if (name === ACCESS_COOKIE) {
            res.cookies.set(ACCESS_COOKIE, value, { httpOnly: true, sameSite: 'lax', secure: COOKIE_SECURE, path: '/' })
        } else if (name === REFRESH_COOKIE) {
            res.cookies.set(REFRESH_COOKIE, value, { httpOnly: true, sameSite: 'strict', secure: COOKIE_SECURE, path: REFRESH_PATH })
        }
    }
    return res
}

/** Only same-origin paths. An open redirect here would be a phishing primitive. */
function safeNext(raw: string | null): string {
    if (!raw || !raw.startsWith('/') || raw.startsWith('//')) return '/'
    return raw
}
