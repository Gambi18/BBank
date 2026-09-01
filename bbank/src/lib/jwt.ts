// ES256 access-token verification.
//
// Deliberately runtime-agnostic: no `server-only`, no `next/headers`, no Node
// built-ins. `proxy.ts` runs on the Edge runtime and server components run on
// Node, and both verify the same token with this module. `jose` uses Web Crypto,
// which both runtimes provide — that is the reason TRD Q7 chose ES256 over
// RS256, and it answers the open question OD-16.
//
// Why the frontend only ever holds the PUBLIC key: with a symmetric algorithm
// the verifying key is also the signing key, so a compromise of this Next.js
// process would become an admin-token factory. Here the worst case is that an
// attacker can read tokens they already have.

import { importSPKI, jwtVerify, type JWTPayload, type KeyObject } from 'jose'
import { API_BASE } from './api'

export const ROLES = ['donor', 'staff', 'lab_tech', 'inventory_manager', 'hospital_user', 'admin'] as const
export type Role = (typeof ROLES)[number]

export const isRole = (v: unknown): v is Role => typeof v === 'string' && (ROLES as readonly string[]).includes(v)

/** The verified claim set (TRD §7.3). Every field here is signed. */
export interface Claims {
    userId: number
    sessionId: string
    role: Role
    /** Home donation centre — `staff` only. Scopes their reads and writes. */
    centerId: number | null
    /** Hospital — `hospital_user` only. */
    hospitalId: number | null
    tokenVersion: number
    expiresAt: number
}

const ISSUER = process.env.JWT_ISSUER || 'https://api.bbank.local'
const AUDIENCE = process.env.JWT_AUDIENCE || 'bbank-web'

// The public key changes only when the keypair is rotated, so it is cached — but
// with a TTL, so a rotation propagates without redeploying the frontend. The API
// serves it with `Cache-Control: max-age=300`; this mirrors that.
const KEY_TTL_MS = 5 * 60 * 1000
let cached: { key: CryptoKey | KeyObject; fetchedAt: number } | null = null
let inFlight: Promise<CryptoKey | KeyObject> | null = null

async function publicKey(): Promise<CryptoKey | KeyObject> {
    if (cached && Date.now() - cached.fetchedAt < KEY_TTL_MS) return cached.key
    // Collapse concurrent misses onto one fetch. Without this, a cold start under
    // load asks the API for the same key once per in-flight request.
    if (inFlight) return inFlight

    inFlight = (async () => {
        const res = await fetch(`${API_BASE}/api/v1/auth/public-key`, { cache: 'no-store' })
        if (!res.ok) throw new Error(`cannot fetch signing key: ${res.status}`)
        const key = await importSPKI(await res.text(), 'ES256')
        cached = { key, fetchedAt: Date.now() }
        return key
    })()

    try {
        return await inFlight
    } finally {
        inFlight = null
    }
}

/**
 * Verifies signature, expiry, issuer and audience, and returns the claims.
 * Returns null for anything that does not verify — a tampered token, an expired
 * one, or a token signed for a different service. The caller cannot tell those
 * apart on purpose: every one of them means "not signed in".
 */
export async function verifyAccessToken(token: string | undefined): Promise<Claims | null> {
    if (!token) return null
    try {
        const { payload } = await jwtVerify(token, await publicKey(), {
            issuer: ISSUER,
            audience: AUDIENCE,
            algorithms: ['ES256'], // pinned: never let the token choose its own algorithm
        })
        return toClaims(payload)
    } catch {
        return null
    }
}

/**
 * Reads the claims WITHOUT verifying the signature. Used in exactly one place —
 * deciding whether an unusable token is merely expired and therefore worth
 * refreshing. It must never gate access to anything.
 */
export function unsafeDecodeExpiry(token: string | undefined): number | null {
    if (!token) return null
    const part = token.split('.')[1]
    if (!part) return null
    try {
        const json = atob(part.replace(/-/g, '+').replace(/_/g, '/'))
        const exp = (JSON.parse(json) as JWTPayload).exp
        return typeof exp === 'number' ? exp : null
    } catch {
        return null
    }
}

function toClaims(p: JWTPayload): Claims | null {
    const userId = Number(p.sub)
    // A role outside the six is not a new role with no rules yet — it is a
    // token we do not understand, and it gets no access at all.
    if (!Number.isInteger(userId) || !isRole(p.role) || typeof p.exp !== 'number') return null
    return {
        userId,
        sessionId: typeof p.sid === 'string' ? p.sid : '',
        role: p.role,
        centerId: typeof p.cid === 'number' ? p.cid : null,
        hospitalId: typeof p.hid === 'number' ? p.hid : null,
        tokenVersion: typeof p.ver === 'number' ? p.ver : 0,
        expiresAt: p.exp,
    }
}
