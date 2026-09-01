import Link from 'next/link'
import { FaArrowRight, FaHelmetSafety } from 'react-icons/fa6'
import { LOGOUT_ACTION } from '@/lib/routes'

/**
 * The landing page for a role whose area is specified but not yet built.
 *
 * These exist because the proxy now understands all six roles (WI-19) while only
 * two of the six areas have pages. Without them a `staff` account would log in
 * successfully and land on a 404, which reads as "the app is broken" rather than
 * "this part is next".
 */
export default function AreaPlaceholder({
    area, workItem, does,
}: {
    area: string
    workItem: string
    does: string[]
}) {
    return (
        <div className="min-h-screen mesh flex items-center justify-center px-6 py-16">
            <div className="w-full max-w-xl card p-10 animate-scale-in">
                <div className="eyebrow">
                    <span className="w-1.5 h-1.5 rounded-full bg-amber-500" /> Not built yet
                </div>

                <h1 className="headline text-3xl mt-4">
                    The <span className="display-serif text-gradient">{area}</span> area is next
                </h1>

                <p className="text-zinc-500 mt-3">
                    Your account is set up and your permissions are live — the API already enforces
                    exactly what this role may do. The screens are scheduled as <code>{workItem}</code>.
                </p>

                <div className="mt-7">
                    <div className="text-sm font-semibold text-zinc-900 flex items-center gap-2">
                        <span className="w-8 h-8 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center text-xs">
                            <FaHelmetSafety />
                        </span>
                        What it will do
                    </div>
                    <ul className="mt-3 flex flex-col gap-2">
                        {does.map((d) => (
                            <li key={d} className="flex gap-3 text-sm text-zinc-600">
                                <span className="mt-1.5 w-1.5 h-1.5 rounded-full bg-rose-300 shrink-0" />
                                {d}
                            </li>
                        ))}
                    </ul>
                </div>

                <div className="flex items-center gap-3 mt-9 pt-6 border-t border-black/5">
                    <form action={LOGOUT_ACTION} method="post">
                        <button type="submit" className="btn btn-ghost">Sign out</button>
                    </form>
                    <Link href="/" className="btn btn-primary ml-auto">
                        Back to the site <FaArrowRight className="text-sm" />
                    </Link>
                </div>
            </div>
        </div>
    )
}
