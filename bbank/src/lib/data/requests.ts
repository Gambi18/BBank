import 'server-only'
import { apiListOrEmpty } from '../apiClient'

export interface DonationRequest {
    id: number
    donor_id: number
    donor_name: string
    last_donation: string
    created_at: string
}

/** Pending requests visible to the caller — their own, their centre's, or all. */
export async function listRequests(): Promise<DonationRequest[]> {
    return (await apiListOrEmpty<DonationRequest>('/api/v1/donation-requests')).items
}
