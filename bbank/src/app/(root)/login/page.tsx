import Link from 'next/link'
import { redirect } from 'next/navigation'
import { FaDroplet, FaArrowRight } from 'react-icons/fa6'
import { api } from '@/lib/api'
import { setSession } from '@/lib/session'

export default function Login() {
    async function handleLogin(formData: FormData) {
        'use server'
        const email = String(formData.get('email') || '').trim().toLowerCase()
        const password = String(formData.get('password') || '')

        // Hardcoded admin login
        if (email === 'admin@admin.com' && password === 'admin') {
            await setSession({ role: 'admin' })
            redirect('/admin')
        }

        // Verify against the backend (bcrypt check happens server-side)
        const res = await fetch(api('/api/go/login'), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, password }),
            cache: 'no-store',
        })

        if (res.ok) {
            const donor = await res.json()
            await setSession({ role: 'donor', id: String(donor.id) })
            redirect(`/donor/${donor.id}`)
        }

        redirect('/login?error=Invalid+email+or+password')
    }

    return (
        <div className='min-h-screen mesh flex items-center justify-center px-6 pt-28 pb-16 relative overflow-hidden'>
            <div className="blob w-96 h-96 bg-rose-100/70 -top-24 -left-24" aria-hidden />
            <div className="w-full max-w-4xl card overflow-hidden grid lg:grid-cols-2 animate-scale-in">
                {/* Brand panel */}
                <div className="hidden lg:flex flex-col justify-between p-10 bg-gradient-to-br from-rose-50 via-white to-amber-50/40 border-r border-black/5">
                    <div className="eyebrow">
                        <span className="w-1.5 h-1.5 rounded-full bg-rose-600 live-dot" /> Welcome back
                    </div>
                    <div>
                        <h1 className="headline text-4xl">
                            Every visit here is <span className="display-serif text-gradient">a life waiting.</span>
                        </h1>
                        <p className="text-zinc-500 text-sm mt-4 max-w-xs">
                            Log back in to check your appointments or request your next donation slot.
                        </p>
                    </div>
                    <div className="flex items-center gap-2 text-zinc-400 text-sm">
                        <FaDroplet className="text-rose-300" /> BloodBank
                    </div>
                </div>

                {/* Form panel */}
                <div className="p-8 lg:p-12 flex flex-col justify-center bg-white">
                    <h2 className="text-2xl font-bold tracking-tight">Log in</h2>
                    <p className="text-zinc-500 text-sm mt-1.5">Enter your credentials to continue.</p>

                    <form action={handleLogin} className="flex flex-col gap-5 mt-8">
                        <div className="animate-fade-up anim-delay-1">
                            <label className="label" htmlFor="email">Email</label>
                            <input id="email" type="email" name="email" placeholder="kofi@example.com" className="field" required autoComplete="email" />
                        </div>
                        <div className="animate-fade-up anim-delay-2">
                            <label className="label" htmlFor="password">Password</label>
                            <input id="password" type="password" name="password" placeholder="••••••••" className="field" required autoComplete="current-password" />
                        </div>
                        <button type='submit' className="btn btn-primary w-full mt-2 animate-fade-up anim-delay-3">
                            Log in <FaArrowRight className="text-sm" />
                        </button>
                    </form>

                    <div className="flex items-center gap-4 my-7 text-zinc-400 text-xs">
                        <span className="flex-1 h-px bg-black/5" /> OR <span className="flex-1 h-px bg-black/5" />
                    </div>
                    <p className="text-sm text-zinc-500 text-center">
                        Don&apos;t have an account?{' '}
                        <Link href="/signup" className="font-semibold text-rose-600 hover:text-rose-700 transition-colors">Sign up</Link>
                    </p>
                </div>
            </div>
        </div>
    )
}
