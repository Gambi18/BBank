import Link from 'next/link'
import { redirect } from 'next/navigation';

async function admin() {
    const appointmentsRes = await fetch('http://localhost:8000/api/go/appointments', { cache: 'no-store' });
    const requestsRes = await fetch('http://localhost:8000/api/go/requests', { cache: 'no-store' });

    const appointments = appointmentsRes.ok ? await appointmentsRes.json() : [];
    const requests = requestsRes.ok ? await requestsRes.json() : [];

    async function createDonor(formData: FormData) {
        'use server'

        const rawFormData = {
            full_name: formData.get('name'),
            email: formData.get('email'),
            password: formData.get('password'),
            dob: formData.get('dob'),
            gender: formData.get('gender'),
            blood_group: formData.get('blood_group'),
            rhesus: formData.get('rhesus'),
            contact: formData.get('contact'),
            address: formData.get('address'),
            last_donation: formData.get('last_donation'),
        }

        const res = await fetch('http://localhost:8000/api/go/donors', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(rawFormData),
        })

        if (!res.ok) {
            console.error('Failed to create donor')
            redirect('/admin?error=Failed+to+create+Patient');
        } else {
            console.log('Donor created successfully')
            redirect('/admin?success=Successfully+created+Patient');
        }
    }


    return (
        <div id="dashboard" className='block gap-5 m-0 h-full justify-center w-full'>
            <h1 id='organization' className='hidden rounded-4xl bg-white text-mist-950 text-center text-2xl uppercase font-bold my-2'>Organization</h1>
            <div className='flex gap-5 h-full justify-center w-full'>
                <div className="left flex flex-col m-0 gap-5 w-1/2 h-full">
                    <Link href="/admin/appointments" id="appointments" className=' bg-slate-800 p-5 rounded-t-2xl h-1/2 hover:bg-slate-700 hover:cursor-pointer'>
                        <h1 className='text-3xl font-bold text-center' >Appointments</h1>
                        <hr />
                        <ul className="appointments-list p-3">
                            {appointments.length > 0 ? (
                                <li className='text-center text-xl font-bold text-white'>{appointments.length} Scheduled</li>
                            ) : (
                                <li className='text-center text-gray-500'>No new appointments</li>
                            )}
                        </ul>
                    </Link>

                    <Link href="/admin/requests" id="requests" className=' bg-slate-800 p-5 rounded-b-2xl h-1/2 hover:bg-slate-700 hover:cursor-pointer'>
                        <h1 className='text-3xl font-bold text-center'>Requests</h1>
                        <hr />
                        <ul className="requests-list p-3">
                            {requests.length > 0 ? (
                                <li className='text-center text-xl font-bold text-white'>{requests.length} Pending</li>
                            ) : (
                                <li className='text-center text-gray-500'>No pending requests</li>
                            )}
                        </ul>
                    </Link>
                </div>

                <div className="right flex rounded-2xl w-1/2">
                    <div id="donors" className=' bg-slate-900 p-5 rounded-2xl h-full'>
                        <h1 className='text-3xl font-bold text-center'>New Donors</h1>
                        <hr />
                        <form action={createDonor} className=''>
                            <input type="text" name="name" id="full_name" placeholder='Name' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                            <input type="email" name="email" id="email" placeholder='Email' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' required/>
                            <input type="password" name="password" id="password" placeholder='Create Password' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                            <div className="flex gap-2">
                                <input type="date" name="dob" id="dob" placeholder='Date of Birth' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' required/>
                                <input type="text" name="gender" id="gender" placeholder='Gender' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' required/>
                            </div>
                            <div className="flex gap-2">
                                <input type="text" name="blood_group" id="blood_group" placeholder='Blood Group' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' required/>
                                <input type="text" name="rhesus" id="rhesus" placeholder='Rhesus' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' required/>
                            </div>
                            <div className="flex gap-2">
                                <input type="text" name="contact" id="contact" placeholder='Contact' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' required/>
                                <input type="text" name="address" id="address" placeholder='Address' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' required/>
                            </div>
                            <label htmlFor="last_donation" className='text-gray-600 mb-0'>Last Donation:</label>
                            <input type="date" name="last_donation" id="last_donation" placeholder='Last Donation' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full mb-5' />
                            <button type="submit" className='bg-red-700 hover:bg-red-800 rounded-md px-7 py-1.5 text-gray-200 font-extrabold align-bottom'>Add Donor</button>
                        </form>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default admin