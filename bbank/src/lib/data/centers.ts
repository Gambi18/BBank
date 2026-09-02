import 'server-only'
import { apiListOrEmpty } from '../apiClient'

/**
 * A donation centre (`WI-24`).
 *
 * The directory carries no PHI — it is the information a centre puts on a
 * poster — which is why `GET /api/v1/centers` is public (TRD §6.5).
 */
export interface Center {
    id: number
    code: string
    name: string
    address_line: string
    city: string
    region: string
    phone?: string | null
    capacity_per_slot: number
    slot_minutes: number
    timezone: string
    is_active: boolean
}

/**
 * Active centres only.
 *
 * The API already filters to active for an anonymous caller and defaults to it
 * for everyone else — a closed centre is not somewhere to go, and offering one
 * in a booking form sends a donor to a locked door.
 */
export async function listCenters(): Promise<Center[]> {
    return (await apiListOrEmpty<Center>('/api/v1/centers')).items
}

export interface Slot {
    starts_at: string
    capacity: number
    booked: number
    available: number
}

/**
 * Bookable slots on one day, capacity minus what is taken.
 *
 * `auth`, not public: how full a centre is on Tuesday is operational detail.
 */
export async function listSlots(centerId: number, date: string): Promise<Slot[]> {
    return (await apiListOrEmpty<Slot>(
        `/api/v1/centers/${centerId}/slots?date=${encodeURIComponent(date)}`,
    )).items
}
