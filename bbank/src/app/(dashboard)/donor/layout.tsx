import React from 'react'
import Link from 'next/link'
import { FaGear } from 'react-icons/fa6'

const layout = ({ children }: { children: React.ReactNode }) => {
    return (
        <div className='inline-flex w-full font-inter bg-linear-180 h-screen bg-white to-red-900 m-0 p-0'>

            {/* Side bar */}
            <div id="sidebar" className='flex flex-col gap-5 bg-slate-700 justify-between py-10 px-10 m-2 rounded-2xl'>
                <div id="logo">
                    <a href="#" className="text-white font-extrabold text-2xl"><span className="text-red-700">Blood</span>Bank</a>
                </div>
                {/* <ul className='flex flex-col gap-11'>
                    <li>
                        <Link href="/donor" className='text-white hover:text-red-600 hover:shadow-xl hover:scale-105 hover:border-b-2 hover:border-red-700'>Dashboard</Link>
                    </li>
                </ul> */}
                <div className="account flex justify-center">
                    <Link href="/donor/settings"><FaGear className='text-white hover:text-red-600 hover:shadow-xl hover:scale-105' /></Link>
                </div>
            </div>

            {/* Main content */}
            <div className="p-20 h-full w-full ">{children}</div>
        </div>
    )
}

export default layout 