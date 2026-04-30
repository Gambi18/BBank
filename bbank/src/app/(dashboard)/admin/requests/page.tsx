import { revalidatePath } from 'next/cache';
import { redirect } from 'next/navigation';

interface Request {
    id: number
    donor_id: number
    donor_name: string
    last_donation: string
    created_at: string
}

async function Requests() {
    const res = await fetch('http://localhost:8000/api/go/requests', { cache: 'no-store' })
    if (!res.ok) {
        throw new Error('Failed to fetch requests')
    }

    const data: Request[] = await res.json()

    async function confirmRequest(formData: FormData) {
        'use server'
        const requestId = formData.get('requestId');
        const date = formData.get('date');

        const res = await fetch(`http://localhost:8000/api/go/requests/${String(requestId)}/confirm`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ date })
        });

        if (res.ok) {
            revalidatePath('/admin/requests');
            revalidatePath('/admin/appointments');
            redirect('/admin/requests?success=Request+successful!!');
        } else {
            redirect('/admin/requests?error=Failed+to+confirm+request');
        }
    }

    return (
        <div className='flex flex-col bg-slate-900 p-5 rounded-2xl h-full'>
            <h1 className='text-3xl font-bold text-center'>Requests</h1>
            <hr className='my-4' />
            <table className='w-full table-auto border-collapse'>
                <thead>
                    <tr className='bg-slate-800 text-left'>
                        <th className='p-2 w-10'>#</th>
                        <th className='p-2'>Donor Name</th>
                        <th className='p-2'>Last Donation</th>
                        <th className='p-2'>Created At</th>
                        <th className='p-2'>Action</th>
                    </tr>
                </thead>
                <tbody>
                    {data.map((request, index) => (
                        <tr key={request.id} className='border-b border-slate-700 hover:bg-slate-800'>
                            <td className='p-2'>{index + 1}</td>
                            <td className='p-2'>{request.donor_name}</td>
                            <td className='p-2'>{request.last_donation}</td>
                            <td className='p-2'>{new Date(request.created_at).toLocaleDateString()}</td>
                            <td className='p-2'>
                                <form action={confirmRequest} className='flex gap-2 items-center'>
                                    <input type="hidden" name="requestId" value={request.id} />
                                    <input 
                                        type="date" 
                                        name="date" 
                                        title="Appointment Date"
                                        className='bg-slate-700 text-white rounded p-1 text-sm' 
                                        required 
                                    />
                                    <button 
                                        type="submit" 
                                        className='bg-green-700 hover:bg-green-800 text-white px-3 py-1 rounded text-sm font-bold'
                                    >
                                        Confirm
                                    </button>
                                </form>
                            </td>
                        </tr>
                    ))}
                    {data.length === 0 && (
                        <tr>
                            <td colSpan={6} className='p-4 text-center text-gray-500'>No pending requests</td>
                        </tr>
                    )}
                </tbody>
            </table>
        </div>
    )
}

export default Requests