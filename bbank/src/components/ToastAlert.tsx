'use client'

import { useSearchParams, useRouter, usePathname } from 'next/navigation'
import { useEffect, useState, Suspense } from 'react'

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

    return (
        <div className={`fixed bottom-5 right-5 p-4 rounded-md shadow-lg text-white font-bold transform transition-all duration-500 z-50 ${visible ? 'translate-y-0 opacity-100' : 'translate-y-10 opacity-0'} ${alert.type === 'success' ? 'bg-green-600' : 'bg-red-600'}`}>
            {alert.message}
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
