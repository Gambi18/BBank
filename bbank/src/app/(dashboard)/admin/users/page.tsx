import { FaUserPlus, FaUsers } from 'react-icons/fa6'
import { listUsers, ROLES } from '@/lib/data/users'
import { inviteUser, setUserStatus, setUserRole } from '@/lib/actions/users'

/**
 * The user console (`WI-18`, `FR-66`).
 *
 * This is what makes removing the hardcoded admin credential an improvement
 * rather than a regression: before it, five of the six roles could only be
 * created with hand-written SQL. `WI-31` expands it (password resets, a
 * confirmation dialog on destructive actions); this is the operational
 * minimum — invite, suspend, reactivate, change role.
 */

// Only the variants defined in globals.css — `badge-success` / `badge-danger`
// do not exist there, and an undefined utility class renders as unstyled text.
const badge = (status: string) =>
    status === 'active' ? 'badge badge-green'
        : status === 'suspended' ? 'badge badge-accent'
            : 'badge badge-muted'

const label = (role: string) => role.replace(/_/g, ' ')

export default async function Users({
    searchParams,
}: {
    searchParams: Promise<{ role?: string; status?: string; q?: string }>
}) {
    const filters = await searchParams
    const { items } = await listUsers(filters)

    async function invite(formData: FormData) {
        'use server'
        await inviteUser(formData)
    }

    // Thin adapters so the actions stay in lib/actions (TRD §4.3) and remain
    // testable without rendering. Editing the hidden id is harmless: the API
    // checks that this caller may administer users, and refuses the
    // last-admin and self-suspension cases regardless of what is submitted.
    async function changeStatus(formData: FormData) {
        'use server'
        await setUserStatus(Number(formData.get('userId')), String(formData.get('status') || ''))
    }

    async function changeRole(formData: FormData) {
        'use server'
        const centre = String(formData.get('center_id') || '').trim()
        await setUserRole(
            Number(formData.get('userId')),
            String(formData.get('role') || ''),
            centre ? Number(centre) : undefined,
        )
    }

    return (
        <div className="animate-fade-up">
            <header className="mb-8">
                <div className="eyebrow">Administration</div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Users</h1>
                <p className="text-zinc-500 mt-2">
                    Invite colleagues, assign roles, and suspend or reactivate accounts.
                    A suspended account stops working on its next request, not at its next login.
                </p>
            </header>

            <section className='card p-8 mb-8'>
                <h2 className="text-lg font-semibold tracking-tight flex items-center gap-2">
                    <span className="w-8 h-8 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center text-sm">
                        <FaUserPlus />
                    </span>
                    Invite someone
                </h2>
                <p className="text-zinc-500 text-sm mt-1.5">
                    They receive a one-time link to set their own password. No password is ever chosen for them.
                </p>

                <form action={invite} className='grid sm:grid-cols-4 gap-4 mt-6'>
                    <div className="sm:col-span-2">
                        <label className="label" htmlFor="email">Email</label>
                        <input id="email" type="email" name="email" placeholder='colleague@hospital.cm' className='field' required />
                    </div>
                    <div>
                        <label className="label" htmlFor="role">Role</label>
                        <select id="role" name="role" className='field' defaultValue="staff" required>
                            {ROLES.map((r) => <option key={r} value={r}>{label(r)}</option>)}
                        </select>
                    </div>
                    <div>
                        {/* Required for staff, forbidden for donors/admins/hospital users —
                            the API says which, in words, rather than failing on a constraint. */}
                        <label className="label" htmlFor="center_id">Centre ID</label>
                        <input id="center_id" type="number" name="center_id" placeholder='e.g. 1' className='field' min={1} />
                    </div>
                    <div className="sm:col-span-4">
                        <button type="submit" className='btn btn-primary'>Create invitation</button>
                    </div>
                </form>
            </section>

            <div className='card overflow-hidden'>
                <div className="overflow-x-auto">
                    <table className='table-modern'>
                        <thead>
                            <tr>
                                <th>Email</th>
                                <th>Role</th>
                                <th>Status</th>
                                <th>Change role</th>
                                <th>Account</th>
                            </tr>
                        </thead>
                        <tbody>
                            {items.map((u) => (
                                <tr key={u.id}>
                                    <td>
                                        <span className="font-medium text-zinc-900">{u.email}</span>
                                        {u.invite_pending && (
                                            <span className="badge badge-muted ml-2">invite pending</span>
                                        )}
                                    </td>
                                    <td className="capitalize">{label(u.role)}</td>
                                    <td><span className={badge(u.status)}>{label(u.status)}</span></td>
                                    <td>
                                        <form action={changeRole} className='flex gap-2 items-center'>
                                            <input type="hidden" name="userId" value={u.id} />
                                            <select name="role" defaultValue={u.role} className='field !w-auto !py-1.5 text-sm'>
                                                {ROLES.map((r) => <option key={r} value={r}>{label(r)}</option>)}
                                            </select>
                                            <input
                                                type="number" name="center_id" min={1} placeholder="Centre"
                                                defaultValue={u.center_id ?? ''} title="Required for staff"
                                                className='field !w-20 !py-1.5 text-sm'
                                            />
                                            <button type="submit" className='btn btn-ghost btn-sm'>Apply</button>
                                        </form>
                                    </td>
                                    <td>
                                        <form action={changeStatus} className='flex gap-2 items-center'>
                                            <input type="hidden" name="userId" value={u.id} />
                                            <input type="hidden" name="status" value={u.status === 'active' ? 'suspended' : 'active'} />
                                            <button
                                                type="submit"
                                                className={`btn btn-sm ${u.status === 'active' ? 'btn-ghost text-rose-700' : 'btn-ghost'}`}
                                            >
                                                {u.status === 'active' ? 'Suspend' : 'Reactivate'}
                                            </button>
                                        </form>
                                    </td>
                                </tr>
                            ))}
                            {items.length === 0 && (
                                <tr>
                                    <td colSpan={5} className="!py-16 text-center">
                                        <div className="flex flex-col items-center gap-3">
                                            <span className="w-12 h-12 rounded-2xl bg-zinc-100 flex items-center justify-center text-zinc-400 text-xl">
                                                <FaUsers />
                                            </span>
                                            <div className="text-zinc-600 font-medium">No accounts yet</div>
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
