import Link from 'next/link'
import { redirect } from 'next/navigation'
import { FaDroplet, FaArrowRight } from 'react-icons/fa6'
import { api } from '@/lib/api'
import { setSession } from '@/lib/session'

export default function Signup() {
    async function handleSignup(formData: FormData) {
        'use server'

        const rawFormData = {
            full_name: formData.get('full_name'),
            email: String(formData.get('email') || '').trim().toLowerCase(),
            password: formData.get('password'),
            dob: formData.get('dob'),
            gender: '',
            blood_group: '',
            rhesus: '',
            contact: '',
            address: '',
            last_donation: '',
        }

        const res = await fetch(api('/api/go/donors'), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(rawFormData),
            cache: 'no-store',
        })

        if (res.ok) {
            const data = await res.json()
            await setSession({ role: 'donor', id: String(data.id) })
            redirect(`/donor/${data.id}?success=Welcome+to+BloodBank!`)
        }

        console.error('Failed to create donor')
        redirect('/signup?error=Failed+to+create+account')
    }

    return (
        <div className='min-h-screen mesh flex items-center justify-center px-6 pt-28 pb-16 relative overflow-hidden'>
            <div className="blob w-96 h-96 bg-rose-100/70 -bottom-24 -right-24" aria-hidden />
            <div className="w-full max-w-4xl card overflow-hidden grid lg:grid-cols-2 animate-scale-in">
                {/* Form panel */}
                <div className="p-8 lg:p-12 flex flex-col justify-center bg-white">
                    <h2 className="text-2xl font-bold tracking-tight">Create your account</h2>
                    <p className="text-zinc-500 text-sm mt-1.5">Join the registry — it takes under a minute.</p>

                    <form action={handleSignup} className="flex flex-col gap-5 mt-8">
                        <div className="animate-fade-up anim-delay-1">
                            <label className="label" htmlFor="full_name">Full name</label>
                            <input id="full_name" type="text" name="full_name" placeholder="Jane Doe" className="field" required autoComplete="name" />
                        </div>
                        <div className="animate-fade-up anim-delay-2">
                            <label className="label" htmlFor="dob">Date of birth</label>
                            <input id="dob" type="date" name="dob" className="field" required />
                        </div>
                        <div className="animate-fade-up anim-delay-3">
                            <label className="label" htmlFor="email">Email</label>
                            <input id="email" type="email" name="email" placeholder="you@example.com" className="field" required autoComplete="email" />
                        </div>
                        <div className="animate-fade-up anim-delay-4">
                            <label className="label" htmlFor="password">Password</label>
                            <input id="password" type="password" name="password" placeholder="••••••••" className="field" required autoComplete="new-password" />
                        </div>
                        <button type='submit' className="btn btn-primary w-full mt-2 animate-fade-up anim-delay-5">
                            Sign up <FaArrowRight className="text-sm" />
                        </button>
                    </form>

                    <div className="flex items-center gap-4 my-7 text-zinc-400 text-xs">
                        <span className="flex-1 h-px bg-black/5" /> OR <span className="flex-1 h-px bg-black/5" />
                    </div>
                    <p className="text-sm text-zinc-500 text-center">
                        Already have an account?{' '}
                        <Link href="/login" className="font-semibold text-rose-600 hover:text-rose-700 transition-colors">Log in</Link>
                    </p>
                </div>

                {/* Brand panel */}
                <div className="hidden lg:flex flex-col justify-between p-10 bg-gradient-to-bl from-rose-50 via-white to-amber-50/40 border-l border-black/5">
                    <div className="eyebrow">
                        <span className="w-1.5 h-1.5 rounded-full bg-rose-600 live-dot" /> Join the registry
                    </div>
                    <div>
                        <h1 className="headline text-4xl">
                            Ten minutes of your day. <span className="display-serif text-gradient">A lifetime for someone else.</span>
                        </h1>
                        <p className="text-zinc-500 text-sm mt-4 max-w-xs">
                            Register once, donate whenever you&apos;re ready. We&apos;ll handle the scheduling.
                        </p>
                    </div>
                    <div className="flex items-center gap-2 text-zinc-400 text-sm">
                        <FaDroplet className="text-rose-300" /> BloodBank
                    </div>
                </div>
            </div>
        </div>
    )
}
