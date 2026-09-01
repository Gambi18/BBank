import Link from 'next/link'
import { FaDroplet, FaArrowRight } from 'react-icons/fa6'
import { acceptInvite } from '@/lib/actions/users'

/**
 * Where an invited colleague sets their password (`WI-18`, `FR-66`).
 *
 * Public by necessity: the invitee has no session yet, which is the point of
 * the invitation. The token in the query string IS the credential, so the page
 * does not verify it up front — doing so would turn page loads into a way to
 * probe for live tokens. It is checked once, on submit, by the API.
 */
export default async function AcceptInvite({
    searchParams,
}: {
    searchParams: Promise<{ token?: string }>
}) {
    const { token } = await searchParams

    return (
        <div className='min-h-screen mesh flex items-center justify-center px-6 pt-28 pb-16 relative overflow-hidden'>
            <div className="blob w-96 h-96 bg-rose-100/70 -top-24 -left-24" aria-hidden />
            <div className="w-full max-w-md card p-8 lg:p-10 animate-scale-in">
                <div className="eyebrow">
                    <span className="w-6 h-6 rounded-lg bg-rose-50 text-rose-600 flex items-center justify-center text-xs">
                        <FaDroplet />
                    </span>
                    Invitation
                </div>

                <h1 className="headline text-2xl mt-4">Set your password</h1>
                <p className="text-zinc-500 text-sm mt-2">
                    You have been invited to BloodBank. Choose a password to finish setting up your account.
                </p>

                {!token ? (
                    <p className="mt-6 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
                        <strong className="font-semibold">This link is incomplete.</strong>{' '}
                        Open the invitation link exactly as you received it, or ask an administrator to send a new one.
                    </p>
                ) : (
                    <form action={acceptInvite} className="flex flex-col gap-5 mt-7">
                        <input type="hidden" name="token" value={token} />
                        <div>
                            <label className="label" htmlFor="password">New password</label>
                            <input
                                id="password" type="password" name="password"
                                placeholder="At least 8 characters" className="field"
                                required minLength={8} autoComplete="new-password"
                            />
                        </div>
                        <div>
                            <label className="label" htmlFor="confirm">Confirm password</label>
                            <input
                                id="confirm" type="password" name="confirm"
                                placeholder="Type it again" className="field"
                                required minLength={8} autoComplete="new-password"
                            />
                        </div>
                        <button type="submit" className="btn btn-primary w-full mt-1">
                            Activate my account <FaArrowRight className="text-sm" />
                        </button>
                    </form>
                )}

                <p className="text-sm text-zinc-500 mt-7">
                    Already set up?{' '}
                    <Link href="/login" className="font-semibold text-rose-700 underline underline-offset-2">Log in</Link>
                </p>
            </div>
        </div>
    )
}
