import 'server-only'
import { apiListOrEmpty } from '../apiClient'

export interface Appointment {
    id: number
    request_id: number
    donor_id: number
    donor_name: string
    appointment_date: string
}

/**
 * The caller's appointments.
 *
 * No `donor_id` parameter: the API scopes the list from the token (WI-20), so a
 * donor gets their own and staff get their centre's. Passing an id here would be
 * asking the server to trust the client about who is asking — the exact shape of
 * the A14 bug.
 */
export async function listAppointments(): Promise<Appointment[]> {
    return (await apiListOrEmpty<Appointment>('/api/go/appointments')).items
}
