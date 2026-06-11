import React from 'react'
import { revalidatePath } from 'next/cache'
import { redirect } from 'next/navigation'
import Link from 'next/link'
import {
    FaUser, FaHeartPulse, FaCalendarCheck, FaDroplet, FaPen, FaArrowRight,
} from 'react-icons/fa6'
import { api } from '@/lib/api'

interface Appointment {
    id: number
    donor_id: number
    donor_name: string
    appointment_date: string
}

const fmtDate = (d: string) =>
    d ? new Date(d).toLocaleDateString(undefined, { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' }) : null

async function DonorDetails({ params }: { params: Promise<{ id: string }> }) {
    const { id } = await params

    // Fetch donor data
    const donorRes = await fetch(api(`/api/go/donors/${id}`), { cache: 'no-store' })
    if (!donorRes.ok) {
        throw new Error(`Failed to fetch donor with ID ${id}`)
    }
    const donorData = await donorRes.json()

    // Fetch appointments for this donor (restricted visibility)
    const appointmentsRes = await fetch(api(`/api/go/appointments?donor_id=${id}`), { cache: 'no-store' })
    let appointments: Appointment[] = []
    if (appointmentsRes.ok) {
        appointments = await appointmentsRes.json()
    }

    async function handleRequest() {
        'use server'
        const res = await fetch(api('/api/go/requests'), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ donor_id: parseInt(id) }),
        })

        if (res.ok) {
            revalidatePath(`/donor/${id}`)
            redirect(`/donor/${id}?success=Request+sent!+We+will+confirm+a+date+soon.`)
        } else {
            console.error(await res.text())
            redirect(`/donor/${id}?error=Failed+to+request+appointment`)
        }
    }

    const firstName = donorData.full_name?.split(' ')[0] || 'Donor'
    const profileIncomplete = !donorData.blood_group || !donorData.contact

    const infoRows = [
        { label: 'Name', value: donorData.full_name },
        { label: 'Email', value: donorData.email },
        { label: 'Date of birth', value: fmtDate(donorData.dob) },
        { label: 'Gender', value: donorData.gender },
        { label: 'Contact', value: donorData.contact },
        { label: 'Address', value: donorData.address },
    ]

    return (
        <div className="animate-fade-up">
            {/* Page header */}
            <header className="mb-10">
                <div className="eyebrow">
                    <span className="w-1.5 h-1.5 rounded-full bg-rose-600 live-dot" /> Donor space
                </div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>
                    Welcome back, <span className="display-serif text-gradient">{firstName}</span>
                </h1>
                <p className="text-zinc-500 mt-2">Your profile, health record and upcoming donations.</p>
            </header>

            {profileIncomplete && (
                <Link href="/donor/settings" className="card card-hover flex items-center justify-between gap-4 p-5 mb-6 !border-rose-200 !bg-rose-50/60 group">
                    <div className="flex items-center gap-3">
                        <span className="w-10 h-10 rounded-xl bg-rose-100 text-rose-600 flex items-center justify-center"><FaPen className="text-sm" /></span>
                        <div>
                            <div className="font-semibold text-rose-700">Complete your profile</div>
                            <div className="text-sm text-zinc-500">Add your blood type and contact so we can match you faster.</div>
                        </div>
                    </div>
                    <FaArrowRight className="text-zinc-300 group-hover:text-rose-600 group-hover:translate-x-1 transition-all duration-200" />
                </Link>
            )}

            <div className='grid lg:grid-cols-2 gap-5'>
                <div className="flex flex-col gap-5">
                    {/* Personal info */}
                    <div className='card card-hover p-7 animate-fade-up anim-delay-1'>
                        <h2 className='font-semibold text-lg flex items-center gap-3'>
                            <span className="w-9 h-9 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center text-sm"><FaUser /></span>
                            Personal information
                        </h2>
                        <ul className="flex flex-col mt-5">
                            {infoRows.map((row) => (
                                <li key={row.label} className="flex justify-between gap-6 py-2.5 border-b border-black/5 last:border-none text-sm">
                                    <span className="text-zinc-500">{row.label}</span>
                                    <span className='text-zinc-900 font-medium text-right'>{row.value || <span className="text-zinc-400 font-normal">Not set</span>}</span>
                                </li>
                            ))}
                        </ul>
                    </div>

                    {/* Health info */}
                    <div className='card card-hover p-7 animate-fade-up anim-delay-2'>
                        <h2 className='font-semibold text-lg flex items-center gap-3'>
                            <span className="w-9 h-9 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center text-sm"><FaHeartPulse /></span>
                            Health information
                        </h2>
                        <ul className="flex flex-col mt-5">
                            <li className="flex justify-between items-center gap-6 py-2.5 border-b border-black/5 text-sm">
                                <span className="text-zinc-500">Blood group</span>
                                {donorData.blood_group
                                    ? <span className="badge badge-accent"><FaDroplet className="text-xs" />{donorData.blood_group} {donorData.rhesus}</span>
                                    : <span className="badge badge-muted">Unknown</span>}
                            </li>
                            <li className="flex justify-between gap-6 py-2.5 border-b border-black/5 text-sm">
                                <span className="text-zinc-500">Rhesus</span>
                                <span className='text-zinc-900 font-medium'>{donorData.rhesus || <span className="text-zinc-400 font-normal">Not set</span>}</span>
                            </li>
                            <li className="flex justify-between gap-6 py-2.5 text-sm">
                                <span className="text-zinc-500">Last donation</span>
                                <span className='text-zinc-900 font-medium'>{fmtDate(donorData.last_donation) || <span className="badge badge-muted">Never donated</span>}</span>
                            </li>
                        </ul>
                    </div>
                </div>

                {/* Appointments */}
                <div className='card p-7 flex flex-col animate-fade-up anim-delay-3'>
                    <h2 className='font-semibold text-lg flex items-center gap-3'>
                        <span className="w-9 h-9 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center text-sm"><FaCalendarCheck /></span>
                        Appointments
                    </h2>
                    <ul className="flex flex-col gap-3 flex-1 overflow-y-auto mt-5">
                        {appointments.length > 0 ? (
                            appointments.map((appt, i) => (
                                <li key={appt.id} className={`card card-hover p-4 !bg-zinc-50 animate-fade-up anim-delay-${Math.min(i + 1, 6)}`}>
                                    <div className="flex items-center justify-between gap-4">
                                        <div>
                                            <div className='text-xs text-zinc-500 uppercase tracking-wider'>Scheduled</div>
                                            <div className='font-semibold mt-0.5'>{fmtDate(appt.appointment_date)}</div>
                                        </div>
                                        <span className="badge badge-green">Confirmed</span>
                                    </div>
                                </li>
                            ))
                        ) : (
                            <li className='flex flex-col flex-1 items-center justify-center text-center py-14'>
                                <span className="w-12 h-12 rounded-2xl bg-zinc-100 flex items-center justify-center text-zinc-400 text-xl mb-3"><FaCalendarCheck /></span>
                                <div className="text-zinc-600 font-medium">No appointments yet</div>
                                <div className="text-zinc-400 text-sm mt-1 max-w-[220px]">Request one below and a coordinator will confirm a date.</div>
                            </li>
                        )}
                    </ul>
                    <form action={handleRequest} className="mt-6">
                        <button type="submit" className='btn btn-primary w-full btn-lg pulse-ring'>
                            Request appointment <FaArrowRight className="text-sm" />
                        </button>
                    </form>
                </div>
            </div>
        </div>
    )
}

export default DonorDetails
