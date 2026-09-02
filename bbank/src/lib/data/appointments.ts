import 'server-only'
import { apiListOrEmpty, type Page } from '../apiClient'

export interface Appointment {
    id: number
    request_id: number
    donor_id: number
    donor_name: string
    center_id: number
    /** scheduled · checked_in · completed · no_show · cancelled · deferred */
    status: string
    scheduled_at: string
    scheduled_date: string
    appointment_date: string
    cancelled_at?: string | null
}

/**
 * The caller's appointments.
 *
 * No `donor_id` parameter: the API scopes the list from the token (WI-20), so a
 * donor gets their own and staff get their centre's. Passing an id here would be
 * asking the server to trust the client about who is asking — the exact shape of
 * the A14 bug.
 */
export async function listAppointments(donorId?: number): Promise<Appointment[]> {
    return (await listAppointmentsPage(donorId)).items
}

/**
 * The same read, keeping the `page` block so callers can show a real total
 * rather than the length of one 25-row page.
 *
 * `donorId` narrows to one donor. It is ANDed with the caller's scope by the
 * API, so it can only ever reduce what is returned — which is what makes it
 * safe to pass from a route parameter. Without it, `/donor/{id}` rendered the
 * *caller's* appointments under that donor's heading: for an admin, every
 * appointment in the system.
 */
export async function listAppointmentsPage(donorId?: number): Promise<{ items: Appointment[]; page?: Page }> {
    const qs = donorId ? `?donor_id=${donorId}` : ''
    return apiListOrEmpty<Appointment>(`/api/v1/appointments${qs}`)
}

/**
 * How an appointment status is shown.
 *
 * Both appointment tables hardcoded a green "Confirmed" badge, so a cancelled
 * or missed appointment displayed as confirmed — the API has returned `status`
 * all along and neither page read it. Only the badge variants defined in
 * `globals.css` are used (`badge-green`, `badge-accent`, `badge-muted`).
 */
export function appointmentBadge(status: string): { label: string; className: string } {
    switch (status) {
        case 'scheduled':
            return { label: 'Confirmed', className: 'badge badge-green' }
        case 'checked_in':
            return { label: 'Checked in', className: 'badge badge-green' }
        case 'completed':
            return { label: 'Completed', className: 'badge badge-green' }
        case 'cancelled':
            return { label: 'Cancelled', className: 'badge badge-muted' }
        case 'no_show':
            return { label: 'Missed', className: 'badge badge-accent' }
        // "Deferred", never "rejected" or "failed": UI/UX §4 reserves those for
        // requests, and this word describes a person.
        case 'deferred':
            return { label: 'Deferred', className: 'badge badge-accent' }
        default:
            return { label: status.replace(/_/g, ' '), className: 'badge badge-muted' }
    }
}
