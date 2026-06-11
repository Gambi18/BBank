import { FaArrowRightFromBracket, FaShieldHalved } from 'react-icons/fa6'
import { logout } from '@/lib/actions'

function AdminSettings() {
    return (
        <div className="max-w-2xl animate-fade-up">
            <header className="mb-8">
                <div className="eyebrow">Account</div>
                <h1 className='headline text-3xl lg:text-4xl mt-3'>Settings</h1>
                <p className="text-zinc-500 mt-2">Manage your admin session.</p>
            </header>

            <div className="card p-8 flex flex-col gap-6">
                <div className="flex items-center gap-4">
                    <span className="w-12 h-12 rounded-2xl bg-rose-50 text-rose-600 flex items-center justify-center text-xl"><FaShieldHalved /></span>
                    <div>
                        <div className="font-semibold text-lg">Administrator</div>
                        <div className="text-zinc-500 text-sm">Full access to donors, requests and appointments.</div>
                    </div>
                </div>

                <hr className="border-black/5" />

                <form action={logout}>
                    <button type="submit" className='btn btn-ghost hover:!border-rose-300 hover:!text-rose-700'>
                        <FaArrowRightFromBracket /> Log out
                    </button>
                </form>
            </div>
        </div>
    )
}

export default AdminSettings
