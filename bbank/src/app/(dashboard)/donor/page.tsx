import React from 'react'

const donor = () => {
    return (
        <div id="dashboard" className='flex gap-5 m-0 h-full justify-center w-full'>
            <div className="left flex flex-col m-0 gap-5 w-1/2 h-full">
                <div id="personal_info" className=' bg-slate-800 p-5 rounded-t-2xl h-1/2'>
                    <h1 className='text-3xl font-bold text-center' >Personal Information</h1>
                    <hr />
                    <ul className="appointments-list flex flex-col gap-3 p-3">
                        <li><span className="text-gray-500">Name:</span></li>
                        <li><span className="text-gray-500">Email:</span></li>
                        <li><span className="text-gray-500">Date of Birth:</span></li>
                        <li><span className="text-gray-500">Gender:</span></li>
                        <li><span className="text-gray-500">Contact:</span></li>
                        <li><span className="text-gray-500">Address:</span></li>
                    </ul>
                </div>

                <div id="health_info" className=' bg-slate-800 p-5 rounded-b-2xl h-1/2'>
                    <h1 className='text-3xl font-bold text-center'>Health Information</h1>
                    <hr />
                    <ul className="requests-list flex flex-col gap-3 p-3">
                        <li className=' self-center'>No new requests</li>
                    </ul>
                </div>
            </div>

            <div className="right flex rounded-2xl w-1/2">
                <div id="appointments" className=' bg-slate-900 p-5 rounded-2xl h-full w-full'>
                    <h1 className='text-3xl font-bold text-center'>Appointments</h1>
                    <hr />
                    <ul id="appointment_list" className="flex flex-col gap-3 p-3 h-full align-center justify-center text-center">
                        <li className='flex flex-col'>
                            <div className='self-center'>No new appointments</div>
                            <button type="button" className=' flex flex-row bg-red-700 text-[0.9rem] hover:bg-red-900 rounded-md px-7 py-1.5 text-gray-200 font-extrabold uppercase self-center'>Request Appointment</button>
                        </li>
                    </ul>
                </div>
            </div>
        </div>
    )
}

export default donor