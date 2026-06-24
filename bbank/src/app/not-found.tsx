import Link from "next/link";
import { FaDroplet, FaArrowLeft } from "react-icons/fa6";

export default function NotFound() {
	return (
		<main className="relative mesh min-h-screen flex items-center justify-center px-6 overflow-hidden">
			<div className="blob w-96 h-96 bg-rose-100/80 -top-24 -right-24" aria-hidden />
			<div className="blob w-72 h-72 bg-amber-50 -bottom-32 -left-20" aria-hidden />
			<div className="relative text-center max-w-lg animate-fade-up">
				<span className="w-16 h-16 rounded-2xl bg-rose-50 text-rose-600 flex items-center justify-center text-2xl mx-auto mb-8">
					<FaDroplet />
				</span>
				<h1 className="headline text-6xl md:text-7xl text-zinc-800">
					404
				</h1>
				<p className="display-serif text-gradient text-2xl mt-2">
					Page not found
				</p>
				<p className="text-zinc-500 mt-5 leading-relaxed">
					This page doesn&apos;t exist or has been moved.
					Let&apos;s get you back to somewhere that does.
				</p>
				<Link href="/" className="btn btn-primary btn-lg mt-10">
					<FaArrowLeft className="text-sm" /> Back home
				</Link>
			</div>
		</main>
	);
}
