import React from 'react'
import Link from 'next/link'
import { FaDroplet, FaEnvelope, FaPhone } from 'react-icons/fa6'
import Navbar from '@/components/Navbar'

const layout = ({ children }: { children: React.ReactNode }) => {
    return (
        <div className='min-h-screen w-full flex flex-col'>
            <Navbar />

            <div className='flex-1'>
                {children}
            </div>

            <footer className="border-t border-black/5 mt-10 bg-white">
                <div className="mx-auto max-w-6xl px-6 py-14 grid grid-cols-2 md:grid-cols-4 gap-10">
                    <div className="col-span-2">
                        <Link href="/" className="flex items-center gap-2.5 font-bold text-lg tracking-tight">
                            <span className="w-8 h-8 rounded-xl bg-rose-600 flex items-center justify-center text-white text-sm">
                                <FaDroplet />
                            </span>
                            <span>Blood<span className="text-rose-600">Bank</span></span>
                        </Link>
                        <p className="text-zinc-500 text-sm mt-4 max-w-xs">
                            Connecting willing donors with hospitals in critical need. One donation can save up to three lives.
                        </p>
                        <div className="flex flex-col gap-2 mt-5 text-sm text-zinc-400">
                            <a href="mailto:hello@bloodbank.example" className="hover:text-rose-600 transition-colors flex items-center gap-2">
                                <FaEnvelope className="text-xs" /> hello@bloodbank.example
                            </a>
                            <a href="tel:+237653532929" className="hover:text-rose-600 transition-colors flex items-center gap-2">
                                <FaPhone className="text-xs" /> +237 6 53 53 29 29
                            </a>
                        </div>
                    </div>
                    <div>
                        <div className="text-sm font-semibold text-zinc-900 mb-4">Platform</div>
                        <ul className="flex flex-col gap-2.5 text-sm text-zinc-500">
                            <li><Link href="/signup" className="hover:text-zinc-900 transition-colors">Become a donor</Link></li>
                            <li><Link href="/login" className="hover:text-zinc-900 transition-colors">Log in</Link></li>
                            <li><Link href="/#how" className="hover:text-zinc-900 transition-colors">How it works</Link></li>
                        </ul>
                    </div>
                    <div>
                        <div className="text-sm font-semibold text-zinc-900 mb-4">Resources</div>
                        <ul className="flex flex-col gap-2.5 text-sm text-zinc-500">
                            <li><Link href="/#about" className="hover:text-zinc-900 transition-colors">About us</Link></li>
                            <li><Link href="/#contact" className="hover:text-zinc-900 transition-colors">Contact</Link></li>
                            <li><a href="https://www.who.int/campaigns/world-blood-donor-day/2018/who-can-give-blood" target="_blank" rel="noreferrer" className="hover:text-zinc-900 transition-colors">Who can donate?</a></li>
                            <li><Link href="/privacy" className="hover:text-zinc-900 transition-colors">Privacy policy</Link></li>
                            <li><Link href="/terms" className="hover:text-zinc-900 transition-colors">Terms of service</Link></li>
                        </ul>
                    </div>
                </div>
                <div className="border-t border-black/5 py-6 text-center text-xs text-zinc-400">
                    &copy; {new Date().getFullYear()} BloodBank. Every drop counts.
                </div>
            </footer>
        </div>
    )
}

export default layout
