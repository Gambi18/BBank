import 'server-only'
import { cookies } from 'next/headers'

export type Session = { role: 'admin' | 'donor'; id?: string }

const COOKIE = 'bb_session'
const MAX_AGE = 60 * 60 * 24 * 7 // 7 days

export async function setSession(session: Session) {
    const store = await cookies()
    store.set(COOKIE, JSON.stringify(session), {
        httpOnly: true,
        sameSite: 'lax',
        secure: process.env.NODE_ENV === 'production',
        path: '/',
        maxAge: MAX_AGE,
    })
}

export async function getSession(): Promise<Session | null> {
    const store = await cookies()
    const raw = store.get(COOKIE)?.value
    if (!raw) return null
    try {
        return JSON.parse(raw) as Session
    } catch {
        return null
    }
}

export async function clearSession() {
    const store = await cookies()
    store.delete(COOKIE)
}
