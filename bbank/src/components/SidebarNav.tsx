'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
    FaDroplet, FaGaugeHigh, FaCalendarCheck, FaInbox, FaUsers,
    FaGear, FaArrowRightFromBracket, FaUser,
} from 'react-icons/fa6'
import { logout } from '@/lib/actions'

type Item = { href: string; label: string; icon: React.ComponentType<{ className?: string }> }

export default function SidebarNav({ role, donorId }: { role: 'admin' | 'donor'; donorId?: string }) {
    const pathname = usePathname()

    const items: Item[] =
        role === 'admin'
            ? [
                  { href: '/admin', label: 'Dashboard', icon: FaGaugeHigh },
                  { href: '/admin/appointments', label: 'Appointments', icon: FaCalendarCheck },
                  { href: '/admin/requests', label: 'Requests', icon: FaInbox },
                  { href: '/admin/donors', label: 'Donors', icon: FaUsers },
              ]
            : [{ href: `/donor/${donorId}`, label: 'My profile', icon: FaUser }]

    const settingsHref = role === 'admin' ? '/admin/settings' : '/donor/settings'

    const isActive = (href: string) =>
        href === '/admin' ? pathname === '/admin' : pathname.startsWith(href)

    return (
        <aside className="flex flex-col w-60 shrink-0 h-screen sticky top-0 border-r border-black/5 bg-white px-4 py-6">
            {/* Logo */}
            <Link href={role === 'admin' ? '/admin' : `/donor/${donorId}`} className="flex items-center gap-2.5 font-bold tracking-tight px-2 mb-8">
                <span className="w-8 h-8 rounded-xl bg-rose-600 flex items-center justify-center text-white text-sm">
                    <FaDroplet />
                </span>
                <span>Blood<span className="text-rose-600">Bank</span></span>
            </Link>

            {/* Section label */}
            <div className="px-3 text-[0.68rem] font-semibold uppercase tracking-[0.14em] text-zinc-400 mb-2">
                {role === 'admin' ? 'Management' : 'Your space'}
            </div>

            {/* Nav */}
            <nav className="flex flex-col gap-1">
                {items.map(({ href, label, icon: Icon }) => {
                    const active = isActive(href)
                    return (
                        <Link
                            key={href}
                            href={href}
                            className={`group flex items-center gap-3 px-3 py-2 rounded-xl text-sm font-medium transition-all duration-200 ${
                                active
                                    ? 'bg-rose-50 text-rose-700 shadow-[inset_0_0_0_1px_#fecdd3]'
                                    : 'text-zinc-500 hover:text-zinc-900 hover:bg-black/[0.04]'
                            }`}
                        >
                            <Icon className={`text-[0.95rem] transition-transform duration-200 group-hover:scale-110 ${active ? 'text-rose-600' : ''}`} />
                            {label}
                            {active && <span className="ml-auto w-1.5 h-1.5 rounded-full bg-rose-600 live-dot" />}
                        </Link>
                    )
                })}
            </nav>

            {/* Bottom */}
            <div className="mt-auto flex flex-col gap-1 pt-4 border-t border-black/5">
                <Link
                    href={settingsHref}
                    className={`group flex items-center gap-3 px-3 py-2 rounded-xl text-sm font-medium transition-all duration-200 ${
                        pathname.startsWith(settingsHref)
                            ? 'bg-rose-50 text-rose-700'
                            : 'text-zinc-500 hover:text-zinc-900 hover:bg-black/[0.04]'
                    }`}
                >
                    <FaGear className="text-[0.95rem] transition-transform duration-300 group-hover:rotate-90" />
                    Settings
                </Link>
                <form action={logout}>
                    <button
                        type="submit"
                        className="w-full flex items-center gap-3 px-3 py-2 rounded-xl text-sm font-medium text-zinc-500 hover:text-rose-700 hover:bg-rose-50 transition-all duration-200"
                    >
                        <FaArrowRightFromBracket className="text-[0.95rem]" />
                        Log out
                    </button>
                </form>
            </div>
        </aside>
    )
}
