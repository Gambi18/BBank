import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { API_BASE } from '@/lib/api'
import { REFRESH_COOKIE, ACCESS_COOKIE, REFRESH_PATH } from '@/lib/session'
import { flash } from '@/lib/flash'

/**
 * Sign out, and actually revoke the refresh-token family server-side.
 *
 * **This route is nested under `/refresh` on purpose. Do not "tidy" it up to
 * `/api/v1/auth/logout`.** `bb_rt` is set with `Path=/api/v1/auth/refresh`
 * (TRD §7.3), and a cookie path matches that path and everything *below* it —
 * so `/api/v1/auth/refresh/logout` receives the cookie and the sibling
 * `/api/v1/auth/logout` would not. Moving this one segment up silently returns
 * us to the bug below.
 *
 * The bug it fixes: logout was a server action, and a server action posts to
 * the page's own URL (`/donor/21`, say). The refresh cookie is not sent there,
 * so the action read `undefined`, skipped the revocation call, and deleted the
 * cookies — leaving the family valid for its full 7-day life. The user saw
 * "You have been signed out" while a stolen refresh token kept working. Logging
 * out has to happen where the token is, and this is the only path that is.
 */
export async function POST(req: NextRequest) {
    const refresh = req.cookies.get(REFRESH_COOKIE)?.value

    if (refresh) {
        try {
            // 204 whether or not the family was still live; a failure here must
            // not strand the user in a session they asked to leave, so the
            // cookies are cleared below either way.
            await fetch(`${API_BASE}/api/v1/auth/logout`, {
                method: 'POST',
                headers: { Cookie: `${REFRESH_COOKIE}=${refresh}` },
                cache: 'no-store',
            })
        } catch {
            // Best effort: the cookies still go, and the family expires on its own.
        }
    }

    const res = NextResponse.redirect(
        new URL(flash('/login', { success: 'You have been signed out' }), req.url),
        // 303: turn the POST into a GET so the browser does not re-post to /login.
        303,
    )
    res.cookies.delete(ACCESS_COOKIE)
    res.cookies.delete({ name: REFRESH_COOKIE, path: REFRESH_PATH })
    return res
}
