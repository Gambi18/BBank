interface Appointment {
    id: number
    donor_id: number
    donor_name: string
    appointment_date: string
}

async function Appointments() {
    const res = await fetch('http://localhost:8000/api/go/appointments', { cache: 'no-store' })
    if (!res.ok) {
        throw new Error('Failed to fetch appointments')
    }

    const data: Appointment[] = await res.json()

    return (
        <div className='flex flex-col bg-slate-900 p-5 rounded-2xl h-full'>
            <h1 className='text-3xl font-bold text-center'>Appointments</h1>
            <hr className='my-4' />
            <table className='w-full table-auto border-collapse'>
                <thead>
                    <tr className='bg-slate-800 text-left'>
                        <th className='p-2'>#</th>
                        <th className='p-2'>Donor Name</th>
                        <th className='p-2'>Donor ID</th>
                        <th className='p-2'>Date</th>
                    </tr>
                </thead>
                <tbody>
                    {data.map((appointment, index) => (
                        <tr key={appointment.id} className='border-b border-slate-700 hover:bg-slate-800'>
                            <td className='p-2'>{index + 1}</td>
                            <td className='p-2'>{appointment.donor_name}</td>
                            <td className='p-2'>{appointment.donor_id}</td>
                            <td className='p-2'>{appointment.appointment_date}</td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    )
}

export default Appointments