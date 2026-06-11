import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

// Lightweight route guard (Next 16 "proxy" convention). The session cookie is
// httpOnly and set on login/signup. (In production this cookie should also be signed/encrypted.)
export function proxy(req: NextRequest) {
    const raw = req.cookies.get('bb_session')?.value
    let session: { role?: string; id?: string } | null = null
    if (raw) {
        try {
            session = JSON.parse(raw)
        } catch {
            session = null
        }
    }

    const { pathname } = req.nextUrl

    // Admin area — admins only
    if (pathname.startsWith('/admin')) {
        if (session?.role !== 'admin') {
            return NextResponse.redirect(new URL('/login?error=Please+log+in', req.url))
        }
    }

    // Donor area — must be signed in, and donors may only see their own page
    if (pathname.startsWith('/donor')) {
        if (!session || (session.role !== 'donor' && session.role !== 'admin')) {
            return NextResponse.redirect(new URL('/login?error=Please+log+in', req.url))
        }
        const match = pathname.match(/^\/donor\/(\d+)/)
        if (match && session.role === 'donor' && session.id !== match[1]) {
            return NextResponse.redirect(new URL(`/donor/${session.id}?error=Access+denied`, req.url))
        }
    }

    return NextResponse.next()
}

export const config = {
    matcher: ['/admin/:path*', '/donor/:path*'],
}
