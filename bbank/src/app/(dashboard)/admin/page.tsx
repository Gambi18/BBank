import Link from 'next/link'
import { FaCalendarCheck, FaInbox, FaUserPlus, FaUsers, FaArrowRight } from 'react-icons/fa6'
import { listDonors } from '@/lib/data/donors'
import { listAppointments } from '@/lib/data/appointments'
import { listRequests } from '@/lib/data/requests'
import { createDonor } from '@/lib/actions/donors'

async function admin() {
    // `limit: 1` because only the count is needed here — the envelope's `total`
    // is the whole registry, so there is no reason to pull every row to call
    // `.length` on it.
    const [appointments, requests, donors] = await Promise.all([
        listAppointments(),
        listRequests(),
        listDonors({ limit: 1 }),
    ])

    const statCards = [
        { href: '/admin/appointments', icon: FaCalendarCheck, label: 'Appointments', value: appointments.length, hint: 'scheduled' },
        { href: '/admin/requests', icon: FaInbox, label: 'Requests', value: requests.length, hint: 'pending review' },
        { href: '/admin/donors', icon: FaUsers, label: 'Donors', value: donors.page?.total ?? donors.items.length, hint: 'in the registry' },
    ]

    return (
        <div className="animate-fade-up">
            {/* Page header */}
            <header className="mb-10">
                <div className="eyebrow">
                    <span className="w-1.5 h-1.5 rounded-full bg-rose-600 live-dot" /> Overview
                </div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Dashboard</h1>
                <p className="text-zinc-500 mt-2">Donations, requests and registry at a glance.</p>
            </header>

            {/* Stats row */}
            <div className="grid sm:grid-cols-3 gap-4 mb-8">
                {statCards.map((s, i) => (
                    <Link key={s.label} href={s.href} className={`card card-hover card-spot p-6 group animate-fade-up anim-delay-${i + 1}`}>
                        <div className="flex items-center justify-between">
                            <span className="w-10 h-10 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center"><s.icon /></span>
                            <FaArrowRight className="text-zinc-300 group-hover:text-rose-600 group-hover:translate-x-1 transition-all duration-200 text-sm" />
                        </div>
                        <div className="text-3xl font-bold tracking-tight mt-4">{s.value}</div>
                        <div className="text-zinc-500 text-sm mt-0.5">{s.label} <span className="text-zinc-600">· {s.hint}</span></div>
                    </Link>
                ))}
            </div>

            {/* Add donor */}
            <div className="card p-8 animate-fade-up anim-delay-4">
                <div className="flex items-center gap-3 mb-6">
                    <span className="w-10 h-10 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center"><FaUserPlus /></span>
                    <div>
                        <h2 className='font-semibold text-lg'>Register a donor</h2>
                        <p className="text-zinc-500 text-sm">Add a walk-in donor directly to the registry.</p>
                    </div>
                </div>

                {/*
                  Live again as of WI-22. Staff and admin MAY set the blood group
                  here — unlike self-registration, where it is ignored — because at
                  the desk it is transcribed from a lab result rather than
                  self-reported (FR-21).
                */}
                <form action={createDonor} className='grid sm:grid-cols-2 gap-5'>
                    <div>
                        <label className="label" htmlFor="name">Full name</label>
                        <input id="name" type="text" name="name" placeholder='Yannick Njiki' className='field' required />
                        </div>
                        <div>
                            <label className="label" htmlFor="email">Email</label>
                            <input id="email" type="email" name="email" placeholder='yannick@example.com' className='field' required />
                    </div>
                    <div>
                        <label className="label" htmlFor="password">Password</label>
                        <input id="password" type="password" name="password" placeholder='Create a password' className='field' required />
                    </div>
                    <div>
                        <label className="label" htmlFor="dob">Date of birth</label>
                        <input id="dob" type="date" name="dob" className='field' required />
                    </div>
                    <div>
                        <label className="label" htmlFor="gender">Gender</label>
                        <select id="gender" name="gender" defaultValue="undisclosed" className='field'>
                            <option value="female">Female</option>
                            <option value="male">Male</option>
                            <option value="other">Other</option>
                            <option value="undisclosed">Prefer not to say</option>
                        </select>
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                        <div>
                            <label className="label" htmlFor="blood_group">Blood group</label>
                            <input id="blood_group" type="text" name="blood_group" placeholder='O' className='field' />
                        </div>
                        <div>
                            <label className="label" htmlFor="rhesus">Rhesus</label>
                            <input id="rhesus" type="text" name="rhesus" placeholder='+' className='field' />
                        </div>
                    </div>
                    <div>
                        <label className="label" htmlFor="contact">Contact</label>
                        <input id="contact" type="text" name="contact" placeholder='Phone number' className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="address">Address</label>
                        <input id="address" type="text" name="address" placeholder='City, street' className='field' />
                    </div>
                    <div>
                        <label className="label" htmlFor="last_donation">Last donation</label>
                        <input id="last_donation" type="date" name="last_donation" className='field' />
                    </div>
                    <div className="flex items-end">
                        <button type="submit" className='btn btn-primary px-8'>Add donor</button>
                    </div>
                </form>
            </div>
        </div>
    )
}

export default admin
