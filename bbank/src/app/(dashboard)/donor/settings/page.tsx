import { redirect } from 'next/navigation'
import { revalidatePath } from 'next/cache'
import { api } from '@/lib/api'
import { getSession } from '@/lib/session'

async function DonorSettings() {
    const session = await getSession()
    if (!session?.id) redirect('/login')

    const res = await fetch(api(`/api/go/donors/${session.id}`), { cache: 'no-store' })
    if (!res.ok) throw new Error('Failed to load profile')
    const d = await res.json()

    async function updateProfile(formData: FormData) {
        'use server'
        const body = {
            full_name: formData.get('full_name'),
            email: formData.get('email'),
            dob: formData.get('dob'),
            gender: formData.get('gender'),
            blood_group: formData.get('blood_group'),
            rhesus: formData.get('rhesus'),
            contact: formData.get('contact'),
            address: formData.get('address'),
            last_donation: formData.get('last_donation'),
            password: formData.get('password') || '', // blank keeps the current password
        }

        const res = await fetch(api(`/api/go/donors/${session!.id}`), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        })

        if (res.ok) {
            revalidatePath(`/donor/${session!.id}`)
            redirect('/donor/settings?success=Profile+updated')
        }
        redirect('/donor/settings?error=Update+failed')
    }

    // Dates may come back as RFC3339 timestamps; <input type=date> needs YYYY-MM-DD.
    const dateOnly = (v: string) => (v ? v.slice(0, 10) : '')

    return (
        <div className="max-w-2xl animate-fade-up">
            <header className="mb-8">
                <div className="eyebrow">Account</div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Profile settings</h1>
                <p className="text-zinc-500 mt-2">Keep your details current — it helps us match you faster.</p>
            </header>

            <div className="card p-8">
                <form action={updateProfile} className='grid sm:grid-cols-2 gap-5'>
                    <div className="sm:col-span-2">
                        <label className="label" htmlFor="full_name">Full name</label>
                        <input id="full_name" type="text" name="full_name" defaultValue={d.full_name} className='field' required />
                    </div>
                    <div className="sm:col-span-2">
                        <label className="label" htmlFor="email">Email</label>
                        <input id="email" type="email" name="email" defaultValue={d.email} className='field' required />
                    </div>
                    <div>
                        <label className="label" htmlFor="dob">Date of birth</label>
                        <input id="dob" type="date" name="dob" defaultValue={dateOnly(d.dob)} className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="gender">Gender</label>
                        <input id="gender" type="text" name="gender" defaultValue={d.gender} placeholder="Gender" className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="blood_group">Blood group</label>
                        <input id="blood_group" type="text" name="blood_group" defaultValue={d.blood_group} placeholder="e.g. O" className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="rhesus">Rhesus</label>
                        <input id="rhesus" type="text" name="rhesus" defaultValue={d.rhesus} placeholder="e.g. +" className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="contact">Contact</label>
                        <input id="contact" type="text" name="contact" defaultValue={d.contact} placeholder="Phone" className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="address">Address</label>
                        <input id="address" type="text" name="address" defaultValue={d.address} placeholder="City, street" className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="last_donation">Last donation</label>
                        <input id="last_donation" type="date" name="last_donation" defaultValue={dateOnly(d.last_donation)} className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="password">New password <span className="text-zinc-600">(optional)</span></label>
                        <input id="password" type="password" name="password" placeholder="Leave blank to keep current" className='field' autoComplete="new-password" />
                    </div>
                    <div className="sm:col-span-2 pt-2">
                        <button type="submit" className='btn btn-primary px-8'>Save changes</button>
                    </div>
                </form>
            </div>
        </div>
    )
}

export default DonorSettings
