import Link from 'next/link'
import { redirect } from 'next/navigation';

interface Donor {
    id: number;
    email: string;
    password: string;
    [key: string]: any;
}

export default function login() {
    async function handleLogin(formData: FormData) {
        'use server'
        const email = formData.get('email');
        const password = formData.get('password');

        // Hardcoded admin login
        if (email === 'admin@admin.com' && password === 'admin') {
            redirect('/admin');
        }

        // Check against donors
        const res = await fetch('http://localhost:8000/api/go/donors', { cache: 'no-store' });
        if (res.ok) {
            const donors = await res.json();
            const donor = donors.find((d: Donor) => d.email === email && d.password === password);
            if (donor) {
                redirect(`/donor/${donor.id}`);
            } else {
                redirect('/login?error=Invalid+email+or+password');
            }
        } else {
            redirect('/login?error=System+error');
        }
    }

    return (
        <div className='lg:h-screen'>
            <div className="flex justify-center items-center text-center rounded lg:h-[calc(100vh-5rem)] w-full">
                <div className="flex w-4/5 bg-slate-700 h-4/6 rounded-2xl">
                    <div className="text lg:w-1/2 hidden lg:block ">
                        <div className="flex justify-center items-center h-full">
                            <h1 className='text-9xl text-red-600 font-black text-center'>LOG IN</h1>
                        </div>
                    </div>

                    <div className="card flex flex-col justify-center items-center bg-slate-900 text-white rounded-2xl p-5 lg:w-1/2">
                        <h2 className="text-xl font-semibold card-title border-b border-red-700 w-max justify-self-center text-center lg:hidden">Login</h2>
                        <form action={handleLogin} className="p-3  text-[1rem]">
                            <input type="email" name="email" placeholder="Email Address" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" required/>
                            <input type="password" name="password" placeholder="Password" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" required/>
                            <button type='submit' className="bg-red-700 hover:bg-red-800 rounded-md px-10 py-2 my-5 border-2 border-red-700 text-gray-200 font-extrabold text-xl">Login</button>
                        </form>
                        <div className='text-[0.8rem]'>-OR-</div>
                        <div className="flex justify-center gap-1 text-[0.9rem]">
                            Don&apos;t have an account? <Link href="/signup" className="font-bold text-red-700 hover:text-red-600">Sign Up</Link>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}
