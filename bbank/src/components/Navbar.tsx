'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { FaDroplet, FaBars, FaXmark } from 'react-icons/fa6'

const links = [
    { href: '/', label: 'Home' },
    { href: '/#about', label: 'About' },
    { href: '/#how', label: 'How it works' },
    { href: '/#contact', label: 'Contact' },
]

export default function Navbar() {
    const [scrolled, setScrolled] = useState(false)
    const [open, setOpen] = useState(false)

    useEffect(() => {
        const onScroll = () => setScrolled(window.scrollY > 12)
        onScroll()
        window.addEventListener('scroll', onScroll, { passive: true })
        return () => window.removeEventListener('scroll', onScroll)
    }, [])

    return (
        <header
            className={`fixed top-0 inset-x-0 z-50 transition-all duration-300 ${
                scrolled ? 'blur-panel !border-x-0 !border-t-0 py-3' : 'bg-transparent py-5'
            }`}
        >
            <nav className="mx-auto max-w-6xl px-6 flex items-center justify-between">
                <Link href="/" className="flex items-center gap-2.5 font-bold text-lg tracking-tight">
                    <span className="w-8 h-8 rounded-xl bg-rose-600 flex items-center justify-center text-white text-sm">
                        <FaDroplet />
                    </span>
                    <span>Blood<span className="text-rose-600">Bank</span></span>
                </Link>

                <div className="hidden md:flex items-center gap-8">
                    {links.map((l) => (
                        <Link key={l.href} href={l.href} className="nav-link">
                            {l.label}
                        </Link>
                    ))}
                </div>

                <div className="hidden md:flex items-center gap-3">
                    <Link href="/login" className="nav-link">Log in</Link>
                    <Link href="/signup" className="btn btn-primary btn-sm">Become a donor</Link>
                </div>

                <button
                    className="md:hidden text-xl p-2 text-zinc-600"
                    onClick={() => setOpen(!open)}
                    aria-label="Toggle menu"
                >
                    {open ? <FaXmark /> : <FaBars />}
                </button>
            </nav>

            {/* Mobile menu */}
            {open && (
                <div className="md:hidden blur-panel !border-x-0 mt-3 px-6 py-5 flex flex-col gap-4 animate-fade-in">
                    {links.map((l) => (
                        <Link key={l.href} href={l.href} className="nav-link text-base" onClick={() => setOpen(false)}>
                            {l.label}
                        </Link>
                    ))}
                    <div className="flex gap-3 pt-2">
                        <Link href="/login" className="btn btn-ghost btn-sm flex-1" onClick={() => setOpen(false)}>Log in</Link>
                        <Link href="/signup" className="btn btn-primary btn-sm flex-1" onClick={() => setOpen(false)}>Become a donor</Link>
                    </div>
                </div>
            )}
        </header>
    )
}
