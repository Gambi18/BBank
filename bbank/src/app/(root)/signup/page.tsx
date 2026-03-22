import Link from 'next/link'

export default function signup() {
    return (
        <div className='lg:h-screen'>
            <div className="flex justify-center items-center text-center rounded lg:h-[calc(100vh-5rem)] w-full">
                <div className="flex w-4/5 bg-slate-700 rounded-2xl">
                    <div className="card bg-slate-900 text-white rounded-2xl p-5 lg:w-1/2">
                        <h2 className="text-xl font-semibold card-title border-b border-red-700 w-max justify-self-center text-center lg:hidden">Sign Up</h2>
                        <form action="submit" className="p-3 text-[1rem]">
                            <div className="flex gap-0 justify-center">
                                <div>
                                    <input type="radio" name="type" id="admin" value="admin" className='hidden peer' required/>
                                    <label htmlFor="admin" className='flex font-bold items-center rounded-l-md px-10 py-2 my-5 text-[.8rem] bg-transparent border-2 border-red-600 text-red-600 cursor-pointer peer-checked:bg-red-600 peer-checked:text-white peer-checked:border-red-600 peer-checked:hover:bg-red-700 peer-checked:hover:border-red-700 transition-all duration-300 ease-in-out'>
                                        <span className='uppercase '>Admin</span>
                                    </label>
                                </div>

                                <div>
                                    <input type="radio" name="type" id="donor" value="donor" className='hidden peer' required/>
                                    <label htmlFor="donor" className='flex font-bold items-center rounded-r-md px-10 py-2 my-5 text-[.8rem] bg-transparent border-2 border-red-600 text-red-600 cursor-pointer peer-checked:bg-red-600 peer-checked:text-white peer-checked:border-red-600 peer-checked:hover:bg-red-700 peer-checked:hover:border-red-700 transition-all duration-300 ease-in-out'>
                                        <span className='uppercase '>Donor</span>
                                    </label>
                                </div>
                            </div>

                            <input type="text" placeholder="Full Name" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" />
                            <input type="date" placeholder="Date of Birth" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" />
                            <input type="email" placeholder="Email Address" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" />
                            <input type="password" placeholder="Password" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" />
                            <button type='submit' className="bg-red-700 hover:bg-red-800 rounded-md px-10 py-2 my-5 border-2 border-red-700 text-gray-200 font-extrabold text-xl">Sign Up</button>
                        </form>
                        <div className='text-[0.8rem]'>-OR-</div>
                        <div className="flex justify-center gap-1 text-[.9rem] mt-5">
                            Already have an account? <Link href="/login" className="font-bold text-red-700 hover:text-red-600">Log In</Link>
                        </div>
                    </div>

                    <div className="text lg:w-1/2 hidden lg:block ">
                        <div className="flex flex-col justify-center items-center h-full">
                            <h1 className='text-9xl text-red-600 font-black text-center'>SIGN UP</h1>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}
