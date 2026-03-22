
const admin = () => {
    return (
        <div id="dashboard" className='block gap-5 m-0 h-full justify-center w-full'>
            <h1 id='organisation' className='hidden rounded-4xl bg-white text-mist-950 text-center text-2xl uppercase font-bold my-2'>Organisation</h1>
            <div className='flex gap-5 h-full justify-center w-full'>
                <div className="left flex flex-col m-0 gap-5 w-1/2 h-full">
                    <div id="appointments" className=' bg-slate-800 p-5 rounded-t-2xl h-1/2'>
                        <h1 className='text-3xl font-bold text-center' >Appointments</h1>
                        <hr />
                        <ul className="appointments-list">
                            <li>No new appointments</li>
                        </ul>
                    </div>

                    <div id="requests" className=' bg-slate-800 p-5 rounded-b-2xl h-1/2'>
                        <h1 className='text-3xl font-bold text-center'>Requests</h1>
                        <hr />
                        <ul className="requests-list">
                            <li>No new requests</li>
                        </ul>
                    </div>
                </div>

                <div className="right flex rounded-2xl w-1/2">
                    <div id="donors" className=' bg-slate-900 p-5 rounded-2xl h-full'>
                        <h1 className='text-3xl font-bold text-center'>New Donors</h1>
                        <hr />
                        <form action="submit" method="post" className=''>
                            <input type="text" name="name" id="" placeholder='Name' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                            <input type="email" name="email" id="" placeholder='Email' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                            <input type="password" name="password" id="" placeholder='Create Password' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                            <div className="flex gap-2">
                                <input type="date" name="dob" id="" placeholder='Date of Birth' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                                <input type="text" name="gender" id="" placeholder='Gender' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                            </div>
                            <div className="flex gap-2">
                                <input type="text" name="blood_group" id="" placeholder='Blood Group' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                                <input type="text" name="rhesus" id="" placeholder='Rhesus' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                            </div>
                            <div className="flex gap-2">
                                <input type="text" name="contact" id="" placeholder='Contact' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                                <input type="text" name="address" id="" placeholder='Address' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5' />
                            </div>
                            <label htmlFor="last_donation" className='text-gray-600 mb-0'>Last Donation:</label>
                            <input type="date" name="last_donation" id="" placeholder='Last Donation' className='bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full mb-5' />
                            <input type="submit" value="Add Donor" className='bg-red-700 hover:bg-red-800 rounded-md px-7 py-1.5 text-gray-200 font-extrabold align-bottom' />
                        </form>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default admin