import { api } from '@/lib/api'

interface Appointment {
    id: number
    donor_id: number
    donor_name: string
    appointment_date: string
}

const initials = (name: string) =>
    name.split(' ').map((p) => p[0]).filter(Boolean).slice(0, 2).join('').toUpperCase() || '?'

const fmtDate = (d: string) =>
    d ? new Date(d).toLocaleDateString(undefined, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric' }) : '—'

async function Appointments() {
    const res = await fetch(api('/api/go/appointments'), { cache: 'no-store' })
    if (!res.ok) {
        throw new Error('Failed to fetch appointments')
    }

    const data: Appointment[] = await res.json()

    return (
        <div className="animate-fade-up">
            <header className="mb-8">
                <div className="eyebrow">Schedule</div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Appointments</h1>
                <p className="text-zinc-500 mt-2">
                    {data.length > 0 ? `${data.length} donation${data.length === 1 ? '' : 's'} on the calendar.` : 'Nothing scheduled yet.'}
                </p>
            </header>

            <div className='card overflow-hidden'>
                <div className="overflow-x-auto">
                    <table className='table-modern'>
                        <thead>
                            <tr>
                                <th>Donor</th>
                                <th>Donor ID</th>
                                <th>Date</th>
                                <th>Status</th>
                            </tr>
                        </thead>
                        <tbody>
                            {data.map((appointment) => (
                                <tr key={appointment.id}>
                                    <td>
                                        <div className="flex items-center gap-3">
                                            <span className="avatar">{initials(appointment.donor_name)}</span>
                                            <span className="font-medium text-zinc-900">{appointment.donor_name}</span>
                                        </div>
                                    </td>
                                    <td className="font-mono text-xs">#{appointment.donor_id}</td>
                                    <td>{fmtDate(appointment.appointment_date)}</td>
                                    <td><span className="badge badge-green">Confirmed</span></td>
                                </tr>
                            ))}
                            {data.length === 0 && (
                                <tr><td colSpan={4} className='!py-12 text-center text-zinc-400'>No appointments scheduled.</td></tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    )
}

export default Appointments
