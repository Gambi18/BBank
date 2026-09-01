'use server'

import { redirect } from 'next/navigation'
import { revalidatePath } from 'next/cache'
import { flash } from '../flash'
import { apiPost, apiPatch, ApiError } from '../apiClient'
import { requireSession } from '../session'

/**
 * Donor writes, restored by `WI-22`.
 *
 * These posted into a 404 between `WI-11` (which moved donors to the layered
 * handlers with reads only) and now, so they were stubbed to fail loudly rather
 * than accept a long form and lose it. They now call
 * `POST /api/v1/register` and `PATCH /api/v1/donors/{id}`.
 */

/** Field names shared by the signup and admin-create forms. */
function readDonorForm(f: FormData) {
    return {
        email: String(f.get('email') || '').trim().toLowerCase(),
        password: String(f.get('password') || ''),
        full_name: String(f.get('full_name') || f.get('name') || '').trim(),
        date_of_birth: String(f.get('date_of_birth') || f.get('dob') || ''),
        gender: String(f.get('gender') || '').trim().toLowerCase() || 'undisclosed',
        contact_phone: String(f.get('contact_phone') || f.get('contact') || '').trim(),
        address_line: String(f.get('address_line') || f.get('address') || '').trim() || undefined,
    }
}

/**
 * Public self-registration.
 *
 * Note what is NOT sent: blood group. It is a laboratory result (`FR-21`), and
 * the API ignores it from a self-registering donor anyway — sending it would
 * only teach the form a lie about who decides it.
 */
export async function signup(formData: FormData) {
    const body = readDonorForm(formData)

    if (!body.email || !body.password || !body.full_name) {
        redirect(flash('/signup', { error: 'Name, email and password are required' }))
    }
    if (body.password.length < 8) {
        redirect(flash('/signup', { error: 'Please choose a password of at least 8 characters' }))
    }

    try {
        // Anonymous by design: this is how someone with no account gets one.
        await apiPost('/api/v1/register', body, { anonymous: true })
    } catch (e) {
        redirect(flash('/signup', { error: describe(e, 'Could not create your account') }))
    }

    // Sign them straight in, so registering does not end at a login form.
    const { login } = await import('./auth')
    const creds = new FormData()
    creds.set('email', body.email)
    creds.set('password', body.password)
    return login(creds)
}

/** Staff/admin creating a donor at the desk. May set the clinical fields. */
export async function createDonor(formData: FormData) {
    await requireSession()
    const body: Record<string, unknown> = readDonorForm(formData)

    const group = String(formData.get('blood_group') || '').trim().toUpperCase()
    const rhesus = String(formData.get('rhesus') || '').trim().toLowerCase()
    // Both or neither: the schema pairs them (donor_profiles_abo_paired), and a
    // half-typed blood type is worse than none.
    if (group && rhesus) {
        body.blood_group = group
        body.rhesus = rhesus === '+' || rhesus.startsWith('pos') ? 'positive' : 'negative'
    }

    if (!body.email || !body.password || !body.full_name) {
        redirect(flash('/admin', { error: 'Name, email and password are required' }))
    }

    try {
        await apiPost('/api/v1/register', body)
    } catch (e) {
        redirect(flash('/admin', { error: describe(e, 'Could not add the donor') }))
    }

    revalidatePath('/admin')
    revalidatePath('/admin/donors')
    redirect(flash('/admin/donors', { success: 'Donor added' }))
}

/**
 * A donor editing their own profile.
 *
 * Blood group is not sent for the same reason as signup, and the API would
 * ignore it regardless: it carries forward whatever the lab recorded rather
 * than blanking it because a form did not include the field.
 */
export async function updateDonorProfile(formData: FormData) {
    const session = await requireSession()
    const body = {
        full_name: String(formData.get('full_name') || '').trim(),
        date_of_birth: String(formData.get('date_of_birth') || ''),
        gender: String(formData.get('gender') || '').trim().toLowerCase() || 'undisclosed',
        contact_phone: String(formData.get('contact_phone') || '').trim(),
        address_line: String(formData.get('address_line') || '').trim() || undefined,
        city: String(formData.get('city') || '').trim() || undefined,
        region: String(formData.get('region') || '').trim() || undefined,
    }

    if (!body.full_name) {
        redirect(flash('/donor/settings', { error: 'Your name cannot be empty' }))
    }

    try {
        await apiPatch(`/api/v1/donors/${session.userId}`, body)
    } catch (e) {
        redirect(flash('/donor/settings', { error: describe(e, 'Could not save your profile') }))
    }

    revalidatePath('/donor/settings')
    revalidatePath(`/donor/${session.userId}`)
    redirect(flash('/donor/settings', { success: 'Profile updated' }))
}

function describe(e: unknown, fallback: string): string {
    if (e instanceof ApiError) {
        // 409 and 422 carry a message written to be read by a person; anything
        // else might carry detail we would rather not put on a page.
        if (e.status === 409 || e.status === 422) return e.message
        if (e.isUnauthenticated) return 'Your session expired. Please log in again.'
        if (e.isForbidden) return 'You do not have permission to do that'
        if (e.isNotFound) return 'That record no longer exists'
    }
    return fallback
}
