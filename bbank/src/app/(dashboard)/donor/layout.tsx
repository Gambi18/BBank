import React from 'react'
import SidebarNav from '@/components/SidebarNav'
import { getSession } from '@/lib/session'

const layout = async ({ children }: { children: React.ReactNode }) => {
    const session = await getSession()
    return (
        <div className='flex w-full min-h-screen'>
            <SidebarNav role="donor" donorId={session ? String(session.userId) : undefined} />
            <main className="flex-1 min-w-0 px-6 py-10 lg:px-12 overflow-y-auto">
                <div className="mx-auto max-w-5xl">{children}</div>
            </main>
        </div>
    )
}

export default layout
