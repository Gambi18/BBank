import React from 'react'
import { FaUsers } from 'react-icons/fa6'
import { listDonors, bloodType } from '@/lib/data/donors'

const initials = (name: string) =>
    name
        .split(' ')
        .map((p) => p[0])
        .filter(Boolean)
        .slice(0, 2)
        .join('')
        .toUpperCase() || '?'

async function Donors() {
    // The layered handler answers with the `{data, page}` envelope, so `total` is
    // the size of the registry rather than the size of this page of it. The old
    // code read `res.json()` as a bare array and quietly rendered nothing.
    const { items: data, page } = await listDonors({ limit: 100 })
    const total = page?.total ?? data.length

    return (
        <div className="animate-fade-up">
            <header className="mb-8">
                <div className="eyebrow">Registry</div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Donors</h1>
                <p className="text-zinc-500 mt-2">{total} {total === 1 ? 'person' : 'people'} registered to give.</p>
            </header>

            <div className='card overflow-hidden'>
                <div className="overflow-x-auto">
                    <table className='table-modern'>
                        <thead>
                            <tr>
                                <th>Donor</th>
                                <th>Contact</th>
                                <th>Blood type</th>
                                <th>Donations</th>
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
                                    <td>{donor.contact_phone || <span className="text-zinc-400">—</span>}</td>
                                    <td>
                                        {bloodType(donor)
                                            ? <span className="badge badge-accent">{bloodType(donor)}</span>
                                            : <span className="badge badge-muted" title="Set by the lab, not self-declared">Untyped</span>}
                                    </td>
                                    <td>
                                        {donor.total_donations > 0
                                            ? `${donor.total_donations} ${donor.total_donations === 1 ? 'donation' : 'donations'}`
                                            : <span className="badge badge-muted">Never</span>}
                                    </td>
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
