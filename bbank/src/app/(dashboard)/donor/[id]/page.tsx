import React from 'react'
import { revalidatePath } from 'next/cache';
import { redirect } from 'next/navigation';

interface Appointment {
    id: number;
    donor_id: number;
    donor_name: string;
    appointment_date: string;
}

async function DonorDetails({ params }: { params: { id: string } }) {
    const { id } = await params;
    
    // Fetch donor data
    const donorRes = await fetch(`http://localhost:8000/api/go/donors/${id}`, { cache: 'no-store' })
    if (!donorRes.ok) {
        throw new Error(`Failed to fetch donor with ID ${id}`)
    }
    const donorData = await donorRes.json()

    // Fetch appointments for this donor (Restricted visibility)
    const appointmentsRes = await fetch(`http://localhost:8000/api/go/appointments?donor_id=${id}`, { cache: 'no-store' })
    let appointments: Appointment[] = [];
    if (appointmentsRes.ok) {
        appointments = await appointmentsRes.json();
    }

    async function handleRequest() {
        'use server'
        const res = await fetch('http://localhost:8000/api/go/requests', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                donor_id: parseInt(id)
            })
        });

        if (res.ok) {
            revalidatePath(`/donor/${id}`);
            redirect(`/donor/${id}?success=Request+successful!!`);
        } else {
            console.error(await res.text());
            redirect(`/donor/${id}?error=Failed+to+request+appointment`);
        }
    }

    return (
        <div id="dashboard" className='flex gap-5 m-0 h-full justify-center w-full'>
            <div className="left flex flex-col m-0 gap-5 w-1/2 h-full">
                <div id="personal_info" className=' bg-slate-800 p-5 rounded-t-2xl h-1/2'>
                    <h1 className='text-3xl font-bold text-center' >Personal Information</h1>
                    <hr />
                    <ul className="appointments-list flex flex-col gap-3 p-3">
                        <li><span className="text-gray-500 text-[0.99rem]">Name:</span> <span className='text-white'>{donorData.full_name}</span></li>
                        <li><span className="text-gray-500 text-[0.99rem]">Email:</span> <span className='text-white'>{donorData.email}</span></li>
                        <li><span className="text-gray-500 text-[0.99rem]">Date of Birth:</span> <span className='text-white'>{donorData.dob}</span></li>
                        <li><span className="text-gray-500 text-[0.99rem]">Gender:</span> <span className='text-white'>{donorData.gender}</span></li>
                        <li><span className="text-gray-500 text-[0.99rem]">Contact:</span> <span className='text-white'>{donorData.contact}</span></li>
                        <li><span className="text-gray-500 text-[0.99rem]">Address:</span> <span className='text-white'>{donorData.address}</span></li>
                    </ul>
                </div>

                <div id="health_info" className=' bg-slate-800 p-5 rounded-b-2xl h-1/2'>
                    <h1 className='text-3xl font-bold text-center'>Health Information</h1>
                    <hr />
                    <ul className="requests-list flex flex-col gap-3 p-3">
                        <li><span className="text-gray-500 text-[0.99rem]">Blood Group:</span> <span className='text-white'>{donorData.blood_group}</span></li>
                        <li><span className="text-gray-500 text-[0.99rem]">Rhesus:</span> <span className='text-white'>{donorData.rhesus}</span></li>
                        <li><span className="text-gray-500 text-[0.99rem]">Last Donation:</span> <span className='text-white'>{donorData.last_donation}</span></li>
                    </ul>
                </div>
            </div>

            <div className="right flex rounded-2xl w-1/2">
                <div id="appointments" className=' bg-slate-900 p-5 rounded-2xl h-full w-full'>
                    <h1 className='text-3xl font-bold text-center'>Appointments</h1>
                    <hr />
                    <ul id="appointment_list" className="flex flex-col gap-3 p-3 h-full overflow-y-auto">
                        {appointments.length > 0 ? (
                            appointments.map((appt) => (
                                <li key={appt.id} className='bg-slate-800 p-3 rounded-lg border border-slate-700'>
                                    <div className='text-gray-400 text-sm'>Date:</div>
                                    <div className='text-white font-bold'>{appt.appointment_date}</div>
                                </li>
                            ))
                        ) : (
                            <li className='flex flex-col h-full align-center justify-center text-center'>
                                <div className='self-center text-gray-500 mb-4'>No scheduled appointments</div>
                            </li>
                        )}
                        <li className='mt-auto self-center'>
                            <form action={handleRequest}>
                                <button type="submit" className='flex flex-row bg-red-700 text-[0.9rem] hover:bg-red-900 rounded-md px-7 py-1.5 text-gray-200 font-extrabold uppercase self-center m-5'>Request Appointment</button>
                            </form>
                        </li>
                    </ul>
                </div>
            </div>
        </div>
    )
}

export default DonorDetails