'use client'

import { useSearchParams, useRouter, usePathname } from 'next/navigation'
import { useEffect, useState, Suspense } from 'react'
import { FaCircleCheck, FaCircleExclamation, FaXmark } from 'react-icons/fa6'

function ToastContent() {
    const searchParams = useSearchParams()
    const router = useRouter()
    const pathname = usePathname()

    const [alert, setAlert] = useState<{ type: 'success' | 'error', message: string } | null>(null)
    const [visible, setVisible] = useState(false)

    useEffect(() => {
        const success = searchParams.get('success')
        const error = searchParams.get('error')

        if (success) {
            setAlert({ type: 'success', message: success })
            setVisible(true)
            cleanParams()
        } else if (error) {
            setAlert({ type: 'error', message: error })
            setVisible(true)
            cleanParams()
        }

        function cleanParams() {
            // Remove the query param so it doesn't show again on refresh
            const newParams = new URLSearchParams(searchParams.toString())
            newParams.delete('success')
            newParams.delete('error')
            const newUrl = `${pathname}${newParams.toString() ? `?${newParams.toString()}` : ''}`
            router.replace(newUrl, { scroll: false })

            // Auto hide after 5 seconds
            setTimeout(() => {
                setVisible(false)
                setTimeout(() => setAlert(null), 500) // wait for fade out animation
            }, 5000)
        }
    }, [searchParams, pathname, router])

    if (!alert) return null

    const isSuccess = alert.type === 'success'

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
                <div className="text-zinc-600">{alert.message}</div>
            </div>
            <button
                onClick={() => { setVisible(false); setTimeout(() => setAlert(null), 500) }}
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
