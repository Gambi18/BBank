'use server'

import { redirect } from 'next/navigation'
import { revalidatePath } from 'next/cache'
import { apiPost, apiPatch, ApiError } from '../apiClient'
import { flash } from '../flash'

/**
 * User administration (`WI-18`, `FR-66`) — invite, suspend, reactivate, change
 * role — plus the public invitation-acceptance flow.
 */

/** Public: the invitee has no session, which is what the token stands in for. */
export async function acceptInvite(formData: FormData) {
    const token = String(formData.get('token') || '')
    const password = String(formData.get('password') || '')
    const confirm = String(formData.get('confirm') || '')

    const back = `/accept-invite?token=${encodeURIComponent(token)}`
    if (!token) redirect(flash('/login', { error: 'That invitation link is incomplete' }))
    if (password.length < 8) {
        redirect(flash(back, { error: 'Please choose a password of at least 8 characters' }))
    }
    if (password !== confirm) {
        // Checked here as well as being a browser concern: a mistyped password
        // on an account you cannot yet log into is a support ticket.
        redirect(flash(back, { error: 'Those passwords do not match' }))
    }

    try {
        await apiPost('/api/v1/invites/accept', { token, password }, { anonymous: true })
    } catch (e) {
        redirect(flash(back, { error: describe(e, 'Could not activate your account') }))
    }

    redirect(flash('/login', { success: 'Your account is ready — please log in' }))
}

export async function inviteUser(formData: FormData) {
    const body: Record<string, unknown> = {
        email: String(formData.get('email') || '').trim().toLowerCase(),
        role: String(formData.get('role') || ''),
    }
    const center = String(formData.get('center_id') || '').trim()
    const hospital = String(formData.get('hospital_id') || '').trim()
    if (center) body.center_id = Number(center)
    if (hospital) body.hospital_id = Number(hospital)

    if (!body.email || !body.role) {
        redirect(flash('/admin/users', { error: 'An email address and a role are required' }))
    }

    let token = ''
    try {
        const data = await apiPost<{ invite_token: string }>('/api/v1/users', body)
        token = data.invite_token
    } catch (e) {
        redirect(flash('/admin/users', { error: describe(e, 'Could not send the invitation') }))
    }

    revalidatePath('/admin/users')
    // The token is shown once, here, because nothing can reproduce it: only its
    // hash is stored. WI-79 replaces this with an email and the success message
    // becomes "invitation sent".
    redirect(flash('/admin/users', {
        success: `Invitation created. Send this link — it is shown only once: /accept-invite?token=${token}`,
    }))
}

export async function setUserStatus(userId: number, status: string) {
    try {
        await apiPatch(`/api/v1/users/${userId}`, { status })
    } catch (e) {
        return redirect(flash('/admin/users', { error: describe(e, 'Could not change that account') }))
    }
    revalidatePath('/admin/users')
    redirect(flash('/admin/users', { success: `Account ${status}` }))
}

export async function setUserRole(userId: number, role: string, centerId?: number) {
    const body: Record<string, unknown> = { role }
    if (centerId) body.center_id = centerId

    try {
        await apiPatch(`/api/v1/users/${userId}`, body)
    } catch (e) {
        return redirect(flash('/admin/users', { error: describe(e, 'Could not change that role') }))
    }
    revalidatePath('/admin/users')
    redirect(flash('/admin/users', { success: `Role changed to ${role}` }))
}

function describe(e: unknown, fallback: string): string {
    if (e instanceof ApiError) {
        // 400/409/422 carry a message written for a person — "staff must be
        // assigned to a donation centre" is more useful than a generic failure.
        if (e.status === 400 || e.status === 409 || e.status === 422) return e.message
        if (e.isUnauthenticated) return 'Your session expired. Please log in again.'
        if (e.isForbidden) return 'You do not have permission to do that'
        if (e.isNotFound) return 'That account no longer exists'
    }
    return fallback
}
