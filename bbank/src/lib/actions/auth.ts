'use server'

import { redirect } from 'next/navigation'
import { API_BASE } from '../api'
import { flash } from '../flash'
import { adoptAuthCookies } from '../session'

/**
 * Log in against the API's own auth endpoint.
 *
 * The hardcoded `admin@admin.com / admin` branch that used to sit at the top of
 * this flow is gone — not because it was tidied away, but because it cannot work
 * any more: a session is now an ES256 token that only the API can sign, and the
 * API signs one only for a real row in `users`. There is no longer a way for the
 * frontend to mint an identity. (`WI-18` still owns replacing it operationally,
 * with an invite flow.)
 */
export async function login(formData: FormData) {
    const email = String(formData.get('email') || '').trim().toLowerCase()
    const password = String(formData.get('password') || '')

    if (!email || !password) {
        redirect(flash('/login', { error: 'Email and password are required' }))
    }

    let res: Response
    try {
        res = await fetch(`${API_BASE}/api/v1/auth/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, password }),
            cache: 'no-store',
        })
    } catch {
        redirect(flash('/login', { error: 'Cannot reach the server. Please try again.' }))
    }

    if (!res.ok) {
        // One message for every failure mode. "No such account" and "wrong
        // password" must be indistinguishable, or the login form becomes a way
        // to enumerate who has an account here (NFR-12).
        const message = res.status === 423
            ? 'This account is temporarily locked'
            : 'Invalid email or password'
        redirect(flash('/login', { error: message }))
    }

    const { data } = (await res.json()) as { data: { user_id: number; role: string } }
    await adoptAuthCookies(res)

    redirect(landingFor(data.role, data.user_id))
}

/** Where a role lands after signing in. Mirrors `homeFor` in the proxy. */
function landingFor(role: string, userId: number): string {
    switch (role) {
        case 'admin': return '/admin'
        case 'staff': return '/staff'
        case 'lab_tech': return '/lab'
        case 'inventory_manager': return '/inventory'
        case 'hospital_user': return '/hospital'
        default: return `/donor/${userId}`
    }
}
