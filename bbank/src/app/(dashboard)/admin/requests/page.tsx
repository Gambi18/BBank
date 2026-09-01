import { FaCheck, FaInbox } from 'react-icons/fa6'
import { listRequests } from '@/lib/data/requests'
import { confirmRequest } from '@/lib/actions/requests'

const initials = (name: string) =>
    name.split(' ').map((p) => p[0]).filter(Boolean).slice(0, 2).join('').toUpperCase() || '?'

const fmtDate = (d: string) => (d ? new Date(d).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }) : null)

async function Requests() {
    const data = await listRequests()

    // Thin adapter: the action itself lives in lib/actions/requests.ts (TRD §4.3)
    // so it can be tested without rendering a page. Editing the hidden field to
    // another request id is harmless — the API checks that this caller may
    // approve that particular request (WI-20), and answers 404 if it is another
    // centre's.
    async function confirm(formData: FormData) {
        'use server'
        const id = Number(formData.get('requestId'))
        await confirmRequest(id, String(formData.get('date') || ''))
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
                                        <form action={confirm} className='flex gap-2 items-center'>
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
                                <tr>
                                    <td colSpan={4} className="!py-16 text-center">
                                        <div className="flex flex-col items-center gap-3">
                                            <span className="w-12 h-12 rounded-2xl bg-zinc-100 flex items-center justify-center text-zinc-400 text-xl">
                                                <FaInbox />
                                            </span>
                                            <div className="text-zinc-600 font-medium">All caught up</div>
                                            <div className="text-zinc-400 text-sm max-w-[220px]">No pending requests. New donation requests will appear here.</div>
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

export default Requests
