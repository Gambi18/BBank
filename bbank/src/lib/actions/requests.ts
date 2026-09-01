'use server'

import { revalidatePath } from 'next/cache'
import { redirect } from 'next/navigation'
import { apiPost, ApiError } from '../apiClient'
import { requireSession } from '../session'
import { flash } from '../flash'

/**
 * Ask for a donation appointment.
 *
 * Note what is NOT passed: a donor id. The API takes it from the `sub` claim of
 * the caller's token (WI-20), so a donor cannot raise a request in someone
 * else's name by editing a form field. The old version sent
 * `{donor_id: parseInt(id)}` read straight out of the URL.
 */
export async function requestAppointment() {
    const session = await requireSession()
    const back = `/donor/${session.userId}`

    try {
        await apiPost('/api/go/requests', {})
    } catch (e) {
        return redirect(flash(back, { error: describe(e, 'Failed to request an appointment') }))
    }

    revalidatePath(back)
    redirect(flash(back, { success: 'Request sent! We will confirm a date soon.' }))
}

/** Approve a pending request and schedule the appointment. Staff and admin only. */
export async function confirmRequest(requestId: number, date: string) {
    if (!date) redirect(flash('/admin/requests', { error: 'Pick a date first' }))

    try {
        await apiPost(`/api/go/requests/${requestId}/confirm`, { date })
    } catch (e) {
        return redirect(flash('/admin/requests', { error: describe(e, 'Failed to confirm the request') }))
    }

    revalidatePath('/admin/requests')
    revalidatePath('/admin/appointments')
    revalidatePath('/admin')
    redirect(flash('/admin/requests', { success: 'Appointment scheduled' }))
}

/**
 * Turns an ApiError into something a person can act on, without leaking why the
 * server said no. A 404 from a scoped read means "not yours or not there" — the
 * API deliberately does not distinguish, and neither should this.
 */
function describe(e: unknown, fallback: string): string {
    if (e instanceof ApiError) {
        if (e.isUnauthenticated) return 'Your session expired. Please log in again.'
        if (e.isForbidden) return 'You do not have permission to do that'
        if (e.isNotFound) return 'That request no longer exists'
    }
    return fallback
}
