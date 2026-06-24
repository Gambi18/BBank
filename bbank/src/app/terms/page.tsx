import Link from "next/link";
import { FaArrowLeft } from "react-icons/fa6";

export default function Terms() {
	return (
		<main className="relative mesh min-h-screen px-6 pt-32 pb-20 overflow-hidden">
			<div className="blob w-96 h-96 bg-rose-100/70 -top-24 -right-24" aria-hidden />
			<div className="relative mx-auto max-w-3xl animate-fade-up">
				<Link href="/" className="btn btn-ghost btn-sm mb-8">
					<FaArrowLeft className="text-xs" /> Back
				</Link>
				<h1 className="headline text-4xl md:text-5xl">Terms of service</h1>
				<p className="text-zinc-500 text-sm mt-2">Last updated: June 2026</p>
				<div className="mt-10 flex flex-col gap-6 text-zinc-600 leading-relaxed">
					<p>
						By using BloodBank, you agree to the following terms. Please read
						them carefully before registering or using our services.
					</p>
					<h2 className="text-zinc-900 font-semibold text-lg">Eligibility</h2>
					<p>
						You must be at least 18 years old and meet the health requirements
						for blood donation as defined by local health authorities to register
						as a donor.
					</p>
					<h2 className="text-zinc-900 font-semibold text-lg">Account responsibility</h2>
					<p>
						You are responsible for maintaining the accuracy of your information
						and for all activity under your account. Notify us immediately of
						any unauthorized use.
					</p>
					<h2 className="text-zinc-900 font-semibold text-lg">Service availability</h2>
					<p>
						We strive to keep BloodBank available at all times, but we do not
						guarantee uninterrupted service. We reserve the right to modify or
						discontinue the service with reasonable notice.
					</p>
					<h2 className="text-zinc-900 font-semibold text-lg">Limitation of liability</h2>
					<p>
						BloodBank is a coordination platform. We are not a medical provider
						and do not offer medical advice. Always consult a healthcare
						professional regarding your eligibility to donate.
					</p>
				</div>
			</div>
		</main>
	);
}
