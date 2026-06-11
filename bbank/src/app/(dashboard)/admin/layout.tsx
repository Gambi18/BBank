import React from 'react'
import SidebarNav from '@/components/SidebarNav'

const layout = ({ children }: { children: React.ReactNode }) => {
    return (
        <div className='flex w-full min-h-screen'>
            <SidebarNav role="admin" />
            <main className="flex-1 min-w-0 px-6 py-10 lg:px-12 overflow-y-auto">
                <div className="mx-auto max-w-5xl">{children}</div>
            </main>
        </div>
    )
}

export default layout
