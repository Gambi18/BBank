import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { verifyAccessToken, unsafeDecodeExpiry, type Role } from '@/lib/jwt'
import { flash } from '@/lib/flash'

/**
 * Route guard (Next 16 "proxy" convention), running on the Edge runtime.
 *
 * It used to `JSON.parse` an unsigned cookie and compare `role` to the two
 * strings it knew. Editing that cookie by hand made you an admin. It now
 * verifies an ES256 signature against the API's public key — which works here
 * because Web Crypto supports P-256 on both the Edge and Node runtimes (TRD Q7,
 * closing OD-16).
 *
 * This is a convenience guard, not the security boundary. The API authorizes
 * every request in its own right (WI-20); the point of checking here is to send
 * someone to the right place instead of showing them a broken page.
 */

/** Which roles may enter each area. An unlisted prefix is not guarded here. */
const AREAS: Array<{ prefix: string; roles: Role[] }> = [
    { prefix: '/admin', roles: ['admin'] },
    { prefix: '/staff', roles: ['staff', 'admin'] },
    { prefix: '/lab', roles: ['lab_tech', 'admin'] },
    { prefix: '/inventory', roles: ['inventory_manager', 'admin'] },
    { prefix: '/hospital', roles: ['hospital_user', 'admin'] },
    { prefix: '/donor', roles: ['donor', 'staff', 'admin'] },
]

/**
 * Where each role belongs when it lands somewhere it may not be. A donor's home
 * is their own record, so it needs their id — there is no bare `/donor` page.
 */
function homeFor(role: Role, userId: number): string {
    switch (role) {
        case 'donor': return `/donor/${userId}`
        case 'staff': return '/staff'
        case 'lab_tech': return '/lab'
        case 'inventory_manager': return '/inventory'
        case 'hospital_user': return '/hospital'
        case 'admin': return '/admin'
    }
}

const REFRESH_PATH = '/api/v1/auth/refresh'

export async function proxy(req: NextRequest) {
    const { pathname, search } = req.nextUrl
    const area = AREAS.find((a) => pathname === a.prefix || pathname.startsWith(`${a.prefix}/`))
    if (!area) return NextResponse.next()

    const token = req.cookies.get('bb_at')?.value
    const session = await verifyAccessToken(token)

    if (!session) {
        // An access token that is merely expired is worth one refresh attempt
        // before signing the user out — that is the entire point of holding a
        // 7-day refresh token. The refresh cookie is path-scoped to the refresh
        // endpoint (TRD §7.3) and so is NOT readable here; redirecting to that
        // path is how the browser is persuaded to send it. One extra round trip,
        // at most once every 15 minutes.
        const exp = unsafeDecodeExpiry(token)
        const expired = exp !== null && exp * 1000 < Date.now()
        if (expired && !req.nextUrl.searchParams.has('_r')) {
            const to = new URL(REFRESH_PATH, req.url)
            to.searchParams.set('next', `${pathname}${search}`)
            return NextResponse.redirect(to)
        }
        return redirectToLogin(req, 'Please log in')
    }

    if (!area.roles.includes(session.role)) {
        // Send them to their own area rather than to the login page: they are
        // signed in, just not here. If their own home is inside the area they
        // were refused — which would mean homeFor is wrong — bounce to login
        // once instead of redirecting in a circle.
        const home = new URL(homeFor(session.role, session.userId), req.url)
        if (home.pathname === pathname || home.pathname.startsWith(`${area.prefix}/`) || home.pathname === area.prefix) {
            return redirectToLogin(req, 'Access denied')
        }
        home.searchParams.set('error', 'You do not have access to that page')
        return NextResponse.redirect(home)
    }

    // A donor may only open their own record. The API returns 404 for anyone
    // else's (WI-20), so this only decides whether they see their own page or an
    // error page — but a redirect is the kinder answer.
    const own = pathname.match(/^\/donor\/(\d+)/)
    if (own && session.role === 'donor' && String(session.userId) !== own[1]) {
        return NextResponse.redirect(new URL(flash(`/donor/${session.userId}`, { error: 'Access denied' }), req.url))
    }

    return NextResponse.next()
}

function redirectToLogin(req: NextRequest, error: string) {
    const to = new URL(flash('/login', { error }), req.url)
    const res = NextResponse.redirect(to)
    // Clear the dead cookie, or every navigation retries the same failed check.
    res.cookies.delete('bb_at')
    return res
}

export const config = {
    matcher: ['/admin/:path*', '/staff/:path*', '/lab/:path*', '/inventory/:path*', '/hospital/:path*', '/donor/:path*'],
}
