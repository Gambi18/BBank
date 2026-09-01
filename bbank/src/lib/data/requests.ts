import 'server-only'
import { apiListOrEmpty } from '../apiClient'

export interface DonationRequest {
    id: number
    donor_id: number
    donor_name: string
    center_id: number
    status: string
    preferred_date: string
    last_donation: string
    created_at: string
    reviewed_at?: string | null
    rejection_reason?: string | null
    notes?: string | null
}

/**
 * Requests visible to the caller — their own, their centre's, or all.
 *
 * The status filter is explicit and defaults to `pending`. It has to be: the
 * v1 endpoint returns every status, whereas the legacy handler it replaced had
 * `WHERE status = 'pending'` welded into its SQL. Without this the review inbox
 * would fill up with already-decided requests and the "waiting for an
 * appointment" count would be wrong (WI-22).
 */
export async function listRequests(status: string | null = 'pending'): Promise<DonationRequest[]> {
    const qs = status ? `?status=${encodeURIComponent(status)}` : ''
    return (await apiListOrEmpty<DonationRequest>(`/api/v1/donation-requests${qs}`)).items
}

export interface RejectionReasonOption {
    value: string
    label: string
}

/**
 * The controlled rejection vocabulary, read from the API rather than hardcoded.
 *
 * A second copy in the frontend is a copy that drifts: the backend would gain a
 * reason and the dropdown would not, or worse, the dropdown would offer one the
 * API rejects. `internal/domain` owns the list; this renders it.
 */
export async function listRejectionReasons(): Promise<RejectionReasonOption[]> {
    const { items } = await apiListOrEmpty<RejectionReasonOption>('/api/v1/donation-requests/rejection-reasons')
    return items
}
