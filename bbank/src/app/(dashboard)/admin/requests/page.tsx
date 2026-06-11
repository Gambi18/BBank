import { revalidatePath } from 'next/cache'
import { redirect } from 'next/navigation'
import { FaCheck } from 'react-icons/fa6'
import { api } from '@/lib/api'

interface Request {
    id: number
    donor_id: number
    donor_name: string
    last_donation: string
    created_at: string
}

const initials = (name: string) =>
    name.split(' ').map((p) => p[0]).filter(Boolean).slice(0, 2).join('').toUpperCase() || '?'

const fmtDate = (d: string) => (d ? new Date(d).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }) : null)

async function Requests() {
    const res = await fetch(api('/api/go/requests'), { cache: 'no-store' })
    if (!res.ok) {
        throw new Error('Failed to fetch requests')
    }

    const data: Request[] = await res.json()

    async function confirmRequest(formData: FormData) {
        'use server'
        const requestId = formData.get('requestId')
        const date = formData.get('date')

        const res = await fetch(api(`/api/go/requests/${String(requestId)}/confirm`), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ date }),
        })

        if (res.ok) {
            revalidatePath('/admin/requests')
            revalidatePath('/admin/appointments')
            redirect('/admin/requests?success=Request+confirmed!')
        } else {
            redirect('/admin/requests?error=Failed+to+confirm+request')
        }
    }

    return (
        <div className="animate-fade-up">
            <header className="mb-8">
                <div className="eyebrow">
                    {data.length > 0 && <span className="w-1.5 h-1.5 rounded-full bg-rose-600 live-dot" />}
                    Inbox
                </div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Requests</h1>
                <p className="text-zinc-500 mt-2">
                    {data.length > 0
                        ? `${data.length} ${data.length === 1 ? 'donor is' : 'donors are'} waiting for an appointment.`
                        : 'All caught up — no pending requests.'}
                </p>
            </header>

            <div className='card overflow-hidden'>
                <div className="overflow-x-auto">
                    <table className='table-modern'>
                        <thead>
                            <tr>
                                <th>Donor</th>
                                <th>Last donation</th>
                                <th>Requested</th>
                                <th>Schedule</th>
                            </tr>
                        </thead>
                        <tbody>
                            {data.map((request) => (
                                <tr key={request.id}>
                                    <td>
                                        <div className="flex items-center gap-3">
                                            <span className="avatar">{initials(request.donor_name)}</span>
                                            <span className="font-medium text-zinc-900">{request.donor_name}</span>
                                        </div>
                                    </td>
                                    <td>{fmtDate(request.last_donation) ?? <span className="badge badge-muted">First time</span>}</td>
                                    <td>{fmtDate(request.created_at) ?? '—'}</td>
                                    <td>
                                        <form action={confirmRequest} className='flex gap-2 items-center'>
                                            <input type="hidden" name="requestId" value={request.id} />
                                            <input
                                                type="date"
                                                name="date"
                                                title="Appointment date"
                                                className='field !w-auto !py-1.5 text-sm'
                                                required
                                            />
                                            <button type="submit" className='btn btn-primary btn-sm'>
                                                <FaCheck className="text-xs" /> Confirm
                                            </button>
                                        </form>
                                    </td>
                                </tr>
                            ))}
                            {data.length === 0 && (
                                <tr><td colSpan={4} className='!py-12 text-center text-zinc-400'>No pending requests.</td></tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    )
}

export default Requests
