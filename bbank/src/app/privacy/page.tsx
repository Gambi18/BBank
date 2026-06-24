import Link from "next/link";
import { FaArrowLeft } from "react-icons/fa6";

export default function Privacy() {
	return (
		<main className="relative mesh min-h-screen px-6 pt-32 pb-20 overflow-hidden">
			<div className="blob w-96 h-96 bg-rose-100/70 -top-24 -right-24" aria-hidden />
			<div className="relative mx-auto max-w-3xl animate-fade-up">
				<Link href="/" className="btn btn-ghost btn-sm mb-8">
					<FaArrowLeft className="text-xs" /> Back
				</Link>
				<h1 className="headline text-4xl md:text-5xl">Privacy policy</h1>
				<p className="text-zinc-500 text-sm mt-2">Last updated: June 2026</p>
				<div className="mt-10 flex flex-col gap-6 text-zinc-600 leading-relaxed">
					<p>
						BloodBank takes your privacy seriously. This policy describes what
						personal data we collect, how we use it, and your rights regarding
						that data.
					</p>
					<h2 className="text-zinc-900 font-semibold text-lg">Information we collect</h2>
					<p>
						When you register as a donor, we collect your name, email address,
						date of birth, blood type, contact information, and donation history.
						This data is necessary to manage the donation process and coordinate
						with hospitals.
					</p>
					<h2 className="text-zinc-900 font-semibold text-lg">How we use your data</h2>
					<p>
						Your data is used exclusively to facilitate blood donations: matching
						donors with appointment slots, notifying you of donation opportunities,
						and maintaining your donation history. We never share your data with
						third parties for marketing purposes.
					</p>
					<h2 className="text-zinc-900 font-semibold text-lg">Data retention</h2>
					<p>
						We retain your information for as long as your account is active.
						You may request deletion of your account and associated data at any
						time by contacting us.
					</p>
				</div>
			</div>
		</main>
	);
}
