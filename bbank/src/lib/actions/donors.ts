'use server'

import { redirect } from 'next/navigation'
import { flash } from '../flash'

/**
 * Creating and editing a donor have no endpoint right now.
 *
 * `POST /donors` and `PUT /donors/{id}` were served by `internal/legacy` against
 * the pre-migration `donors` table. When donors moved to the layered handlers
 * (WI-11) only the reads came across, so these two have been dead since then —
 * not a regression from the session migration. `WI-22` restores them as
 * `POST`/`PATCH /api/v1/donors`, writing to `users` + `donor_profiles`.
 *
 * They fail loudly instead of posting into a 404, so nobody fills in a long form
 * and is told it worked.
 */

export async function createDonor(): Promise<void> {
    redirect(flash('/admin', { error: 'Adding a donor is not available yet — see WI-22' }))
}

export async function updateDonorProfile(): Promise<void> {
    redirect(flash('/donor/settings', { error: 'Editing your profile is not available yet — see WI-22' }))
}

export async function signup(): Promise<void> {
    redirect(flash('/signup', { error: 'Self signup is temporarily unavailable — see WI-22' }))
}
