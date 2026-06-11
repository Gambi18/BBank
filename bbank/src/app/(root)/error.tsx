'use client';

import { useEffect } from 'react'
import { FaTriangleExclamation } from 'react-icons/fa6'

export default function Error({ error, reset }: { error: Error & { digest?: string }, reset: () => void }) {
    useEffect(() => {
        console.error(error)
    }, [error])

    return (
        <div className="min-h-screen mesh flex flex-col items-center justify-center text-center px-6 animate-fade-up">
            <span className="w-16 h-16 rounded-2xl bg-rose-50 text-rose-600 flex items-center justify-center text-2xl mb-6 pulse-ring">
                <FaTriangleExclamation />
            </span>
            <h1 className="headline text-3xl md:text-4xl">
                Something went <span className="display-serif text-gradient">wrong</span>
            </h1>
            <p className="text-zinc-500 mt-3 max-w-sm">
                An unexpected error occurred. It&apos;s not you — try again, or come back in a moment.
            </p>
            <button onClick={() => reset()} className="btn btn-primary mt-8 px-8">Try again</button>
        </div>
    )
}
