import React from 'react'
import Link from 'next/link'

const layout = ({ children }: { children: React.ReactNode }) => {


    return (
        <main className='flex-center min-h-screen w-full font-inter'>
            <nav className="flex nav-bar justify-between items-center bg-white p-5">
                <div className="logo">
                    <a href="#" className="text-black font-extrabold text-2xl"><span className="text-red-700">Blood</span>Bank</a>
                </div>
                <div className="flex gap-10">
                    <Link href="#" className="nav-link text-black hover:text-red-600 hover:shadow-xl hover:scale-105 hover:border-b-2 hover:border-red-700">Home</Link>
                    <Link href="#about" className="nav-link text-black hover:text-red-600 hover:shadow-xl hover:scale-105 hover:border-b-2 hover:border-red-700">About</Link>
                    <Link href="#contact" className="nav-link text-black hover:text-red-600 hover:shadow-xl hover:scale-105 hover:border-b-2 hover:border-red-700">Contacts</Link>
                    <Link href="https://www.who.int/campaigns/world-blood-donor-day/2018/who-can-give-blood" className="nav-link text-black hover:text-red-600 hover:shadow-xl hover:scale-105 hover:border-b-2 hover:border-red-700">Who can Donate?</Link>
                </div>
                <div className="account">
                    <a href="#" className="nav-link"><button className=" bg-red-700 hover:bg-red-800 rounded-full px-7 py-1.5 text-gray-200 font-extrabold">Donate</button></a>
                </div>
            </nav>

            <div className=' bg-linear-180 h-[calc(100vh-5rem)] bg-white to-red-900'>
                {children}
            </div>
        </main>
    )
}

export default layout