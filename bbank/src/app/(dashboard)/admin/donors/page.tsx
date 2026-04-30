import React from 'react'

interface Donor {
    id: number
    full_name: string
    email: string
    contact: string
    blood_group: string
    rhesus: string
    last_donation: string
}

async function Donors() {
    const res = await fetch('http://localhost:8000/api/go/donors', { cache: 'no-store' })
    if (!res.ok) {
        throw new Error('Failed to fetch donors')
    }

    const data: Donor[] = await res.json()

    return (
        <div className='flex flex-col bg-slate-900 p-5 rounded-2xl h-full'>
            <h1 className='text-3xl font-bold text-center text-white'>Donors</h1>
            <hr className='my-4' />
            <table className='w-full table-auto border-collapse'>
                <thead>
                    <tr className='bg-slate-800 text-left'>
                        <th className='p-2'>#</th>
                        <th className='p-2'>Name</th>
                        <th className='p-2'>Email</th>
                        <th className='p-2'>Phone</th>
                        <th className='p-2'>Blood Group</th>
                        <th className='p-2'>Last Donation</th>
                    </tr>
                </thead>
                <tbody>
                    {data.map((donor, index) => (
                        <tr key={donor.id} className='border-b border-slate-700 hover:bg-slate-800'>
                            <td className='p-2'>{index + 1}</td>
                            <td className='p-2'>{donor.full_name}</td>
                            <td className='p-2'>{donor.email}</td>
                            <td className='p-2'>{donor.contact}</td>
                            <td className='p-2'>{donor.blood_group} {donor.rhesus}</td>
                            <td className='p-2'>{donor.last_donation || 'N/A'}</td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    )
}

export default Donors