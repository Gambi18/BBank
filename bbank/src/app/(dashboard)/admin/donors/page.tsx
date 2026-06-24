import React from 'react'
import { FaUsers } from 'react-icons/fa6'
import { api } from '@/lib/api'

interface Donor {
    id: number
    full_name: string
    email: string
    contact: string
    blood_group: string
    rhesus: string
    last_donation: string
}

const initials = (name: string) =>
    name
        .split(' ')
        .map((p) => p[0])
        .filter(Boolean)
        .slice(0, 2)
        .join('')
        .toUpperCase() || '?'

const fmtDate = (d: string) => (d ? new Date(d).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }) : null)

async function Donors() {
    const res = await fetch(api('/api/go/donors'), { cache: 'no-store' })
    if (!res.ok) {
        throw new Error('Failed to fetch donors')
    }

    const data: Donor[] = await res.json()

    return (
        <div className="animate-fade-up">
            <header className="mb-8">
                <div className="eyebrow">Registry</div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Donors</h1>
                <p className="text-zinc-500 mt-2">{data.length} {data.length === 1 ? 'person' : 'people'} registered to give.</p>
            </header>

            <div className='card overflow-hidden'>
                <div className="overflow-x-auto">
                    <table className='table-modern'>
                        <thead>
                            <tr>
                                <th>Donor</th>
                                <th>Contact</th>
                                <th>Blood type</th>
                                <th>Last donation</th>
                            </tr>
                        </thead>
                        <tbody>
                            {data.map((donor) => (
                                <tr key={donor.id}>
                                    <td>
                                        <div className="flex items-center gap-3">
                                            <span className="avatar">{initials(donor.full_name)}</span>
                                            <div>
                                                <div className="font-medium text-zinc-900">{donor.full_name}</div>
                                                <div className="text-xs text-zinc-500">{donor.email}</div>
                                            </div>
                                        </div>
                                    </td>
                                    <td>{donor.contact || <span className="text-zinc-400">—</span>}</td>
                                    <td>
                                        <span className="badge badge-accent">{donor.blood_group || '?'}{donor.rhesus === 'Positive' || donor.rhesus === '+' ? '+' : donor.rhesus === 'Negative' || donor.rhesus === '−' || donor.rhesus === '-' ? '−' : ` ${donor.rhesus}`}</span>
                                    </td>
                                    <td>{fmtDate(donor.last_donation) ?? <span className="badge badge-muted">Never</span>}</td>
                                </tr>
                            ))}
                            {data.length === 0 && (
                                <tr>
                                    <td colSpan={4} className="!py-16 text-center">
                                        <div className="flex flex-col items-center gap-3">
                                            <span className="w-12 h-12 rounded-2xl bg-zinc-100 flex items-center justify-center text-zinc-400 text-xl">
                                                <FaUsers />
                                            </span>
                                            <div className="text-zinc-600 font-medium">No donors yet</div>
                                            <div className="text-zinc-400 text-sm max-w-[220px]">Donors appear here once they sign up or an admin registers them.</div>
                                        </div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    )
}

export default Donors
