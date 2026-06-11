'use client'

import { useEffect, useRef } from 'react'

// Scroll-triggered reveal. Wrap any block; it fades/slides in when it enters
// the viewport. `delay` (seconds) staggers siblings.
export default function Reveal({
    children,
    delay = 0,
    className = '',
}: {
    children: React.ReactNode
    delay?: number
    className?: string
}) {
    const ref = useRef<HTMLDivElement>(null)

    useEffect(() => {
        const el = ref.current
        if (!el) return
        const observer = new IntersectionObserver(
            (entries) => {
                for (const entry of entries) {
                    if (entry.isIntersecting) {
                        entry.target.classList.add('revealed')
                        observer.unobserve(entry.target)
                    }
                }
            },
            { threshold: 0.12, rootMargin: '0px 0px -40px 0px' }
        )
        observer.observe(el)
        return () => observer.disconnect()
    }, [])

    return (
        <div
            ref={ref}
            className={`reveal ${className}`}
            style={{ '--reveal-delay': `${delay}s` } as React.CSSProperties}
        >
            {children}
        </div>
    )
}
