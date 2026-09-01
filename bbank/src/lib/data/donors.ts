import 'server-only'
import { apiGet, apiListOrEmpty, type Page } from '../apiClient'

/**
 * Donor reads.
 *
 * Separate from `lib/actions/donors.ts` because that module is `'use server'`,
 * where every export becomes a callable server action — a POST endpoint. Reads
 * must not be exposed that way, so they live here instead.
 *
 * The shapes below are the layered handler's DTOs (`internal/http/dto`), not the
 * pre-migration `donors` table's columns. `contact_phone` and `total_donations`
 * replace the old `contact` and `last_donation`; pages were still reading the
 * old names and rendering blanks.
 */

export interface DonorSummary {
    id: number
    email: string
    full_name: string
    blood_group: string | null
    rhesus: string | null
    contact_phone: string | null
    total_donations: number
    status: string
}

export interface DonorDetail extends DonorSummary {
    date_of_birth?: string | null
    gender?: string | null
    address_line?: string | null
    last_donation_at?: string | null
    legacy_last_donation?: string | null
}

/** The donor's blood type as one string, or null when the lab has not typed them. */
export const bloodType = (d: { blood_group?: string | null; rhesus?: string | null }): string | null =>
    d.blood_group && d.rhesus ? `${d.blood_group}${d.rhesus === 'positive' ? '+' : '-'}` : null

export async function listDonors(params?: { search?: string; limit?: number; offset?: number }): Promise<{ items: DonorSummary[]; page?: Page }> {
    const q = new URLSearchParams()
    if (params?.search) q.set('search', params.search)
    if (params?.limit !== undefined) q.set('limit', String(params.limit))
    if (params?.offset !== undefined) q.set('offset', String(params.offset))
    const qs = q.toString()
    return apiListOrEmpty<DonorSummary>(`/api/go/donors${qs ? `?${qs}` : ''}`)
}

export async function getDonor(id: number | string): Promise<DonorDetail> {
    return apiGet<DonorDetail>(`/api/go/donors/${id}`)
}
