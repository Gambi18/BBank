'use client'

import { useSearchParams, useRouter, usePathname } from 'next/navigation'
import { useRef, useState, useEffect, Suspense } from 'react'
import { FaCircleCheck, FaCircleExclamation, FaXmark } from 'react-icons/fa6'

function ToastContent() {
    const searchParams = useSearchParams()
    const router = useRouter()
    const pathname = usePathname()

    const success = searchParams.get('success')
    const error = searchParams.get('error')

    const [current, setCurrent] = useState<{ type: 'success' | 'error'; message: string } | null>(null)
    const [visible, setVisible] = useState(false)
    const cleaned = useRef(false)

    useEffect(() => {
        if (cleaned.current || (!success && !error)) return
        cleaned.current = true

        const kind = success ? 'success' : 'error'
        const msg = success || error || ''

        const id = setTimeout(() => {
            setCurrent({ type: kind, message: msg })
            setVisible(true)
        }, 0)

        const params = new URLSearchParams(searchParams.toString())
        params.delete('success')
        params.delete('error')
        const next = `${pathname}${params.toString() ? '?' + params.toString() : ''}`
        router.replace(next, { scroll: false })

        const hideId = setTimeout(() => {
            setVisible(false)
            setTimeout(() => setCurrent(null), 500)
        }, 5000)

        return () => { clearTimeout(id); clearTimeout(hideId) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [success, error])

    if (!current) return null

    const isSuccess = current.type === 'success'

    return (
        <div
            role="status"
            className={`fixed bottom-6 right-6 z-[100] flex items-center gap-3 blur-panel rounded-2xl pl-4 pr-3 py-3.5 shadow-xl shadow-black/10
                transition-all duration-500 [transition-timing-function:cubic-bezier(0.22,1,0.36,1)]
                ${visible ? 'translate-y-0 opacity-100' : 'translate-y-6 opacity-0'}
                ${isSuccess ? '!border-emerald-200' : '!border-rose-200'}`}
        >
            <span className={`w-8 h-8 rounded-xl flex items-center justify-center shrink-0 ${isSuccess ? 'bg-emerald-50 text-emerald-600' : 'bg-rose-50 text-rose-600'}`}>
                {isSuccess ? <FaCircleCheck /> : <FaCircleExclamation />}
            </span>
            <div className="text-sm">
                <div className={`font-semibold ${isSuccess ? 'text-emerald-700' : 'text-rose-700'}`}>
                    {isSuccess ? 'Success' : 'Something went wrong'}
                </div>
                <div className="text-zinc-600">{current.message}</div>
            </div>
            <button
                onClick={() => { setVisible(false); setTimeout(() => setCurrent(null), 500) }}
                className="ml-2 p-1.5 rounded-lg text-zinc-400 hover:text-zinc-700 hover:bg-black/5 transition-colors"
                aria-label="Dismiss"
            >
                <FaXmark className="text-sm" />
            </button>
        </div>
    )
}

export default function ToastAlert() {
    return (
        <Suspense fallback={null}>
            <ToastContent />
        </Suspense>
    )
}