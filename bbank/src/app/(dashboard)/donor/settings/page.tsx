import { requireSession } from '@/lib/session'
import { getDonor, bloodType } from '@/lib/data/donors'
import { updateDonorProfile } from '@/lib/actions/donors'

async function DonorSettings() {
    // The id comes from the verified token, never from the URL — and the API
    // would return 404 for anyone else's record anyway (WI-20).
    const session = await requireSession()
    const d = await getDonor(session.userId)

    // Dates may come back as RFC3339 timestamps; <input type=date> needs YYYY-MM-DD.
    const dateOnly = (v?: string | null) => (v ? v.slice(0, 10) : '')

    return (
        <div className="max-w-2xl animate-fade-up">
            <header className="mb-8">
                <div className="eyebrow">Account</div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Profile settings</h1>
                <p className="text-zinc-500 mt-2">Keep your details current — it helps us match you faster.</p>
            </header>

            <div className="card p-8">
                {/*
                  Live again as of WI-22: PATCH /api/v1/donors/{id}. Ownership comes
                  from the token, so this saves your own profile and nobody else's
                  regardless of what the URL says. Blood group is absent on purpose —
                  it is a lab result (FR-21), and the API carries the recorded value
                  forward rather than letting this form blank it.
                */}
                <form action={updateDonorProfile} className='grid sm:grid-cols-2 gap-5'>
                    <div className="sm:col-span-2">
                        <label className="label" htmlFor="full_name">Full name</label>
                        <input id="full_name" type="text" name="full_name" defaultValue={d.full_name} className='field' required />
                    </div>
                    <div className="sm:col-span-2">
                        <label className="label" htmlFor="email">Email</label>
                        <input id="email" type="email" name="email" defaultValue={d.email} className='field' required />
                    </div>
                    <div>
                        <label className="label" htmlFor="date_of_birth">Date of birth</label>
                        <input id="date_of_birth" type="date" name="date_of_birth" defaultValue={dateOnly(d.date_of_birth)} className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="gender">Gender</label>
                        <select id="gender" name="gender" defaultValue={d.gender ?? 'undisclosed'} className='field'>
                            <option value="female">Female</option>
                            <option value="male">Male</option>
                            <option value="other">Other</option>
                            <option value="undisclosed">Prefer not to say</option>
                        </select>
                    </div>
                    <div>
                        <label className="label" htmlFor="contact_phone">Contact</label>
                        <input id="contact_phone" type="tel" name="contact_phone" defaultValue={d.contact_phone ?? ''} placeholder="Phone" className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="address_line">Address</label>
                        <input id="address_line" type="text" name="address_line" defaultValue={d.address_line ?? ''} placeholder="City, street" className='field' />
                    </div>
                    <div className="sm:col-span-2 pt-2">
                        <button type="submit" className='btn btn-primary px-8'>Save changes</button>
                    </div>
                </form>

                {/*
                  Blood group, rhesus and donation count are deliberately NOT fields
                  on this form. TRD §7.7: they are clinical facts set by staff and
                  the lab, not something a donor declares about themselves. They are
                  shown here, but only as facts.
                */}
                <dl className="mt-8 pt-6 border-t border-black/5 grid sm:grid-cols-3 gap-4 text-sm">
                    <div>
                        <dt className="text-zinc-500">Blood type</dt>
                        <dd className="font-medium text-zinc-900 mt-0.5">{bloodType(d) ?? 'Not yet typed'}</dd>
                    </div>
                    <div>
                        <dt className="text-zinc-500">Donations</dt>
                        <dd className="font-medium text-zinc-900 mt-0.5">{d.total_donations}</dd>
                    </div>
                    <div>
                        <dt className="text-zinc-500">Status</dt>
                        <dd className="font-medium text-zinc-900 mt-0.5 capitalize">{d.status?.replace(/_/g, ' ')}</dd>
                    </div>
                </dl>
                <p className="mt-3 text-xs text-zinc-400">
                    Set by the clinical team — these are not self-declared.
                </p>
            </div>
        </div>
    )
}

export default DonorSettings
