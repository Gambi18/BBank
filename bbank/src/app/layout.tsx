import type { Metadata } from "next";
import { Outfit, Geist_Mono, Instrument_Serif } from "next/font/google";
import "./globals.css";
import ToastAlert from "@/components/ToastAlert";

const outfit = Outfit({
	variable: "--font-outfit",
	subsets: ["latin"],
});

const geistMono = Geist_Mono({
	variable: "--font-geist-mono",
	subsets: ["latin"],
});

const instrumentSerif = Instrument_Serif({
	variable: "--font-display",
	weight: "400",
	style: ["normal", "italic"],
	subsets: ["latin"],
});

export const metadata: Metadata = {
	title: "BloodBank — Donate blood, save lives",
	description:
		"A modern blood bank management platform connecting donors with hospitals in need.",
	openGraph: {
		title: "BloodBank — Donate blood, save lives",
		description:
			"A modern blood bank management platform connecting donors with hospitals in need.",
		type: "website",
	},
};

export default function RootLayout({
	children,
}: Readonly<{
	children: React.ReactNode;
}>) {
	return (
		<html lang="en">
			<body
				className={`${outfit.variable} ${geistMono.variable} ${instrumentSerif.variable} antialiased`}
			>
				<a
					href="#main-content"
					className="sr-only focus:not-sr-only focus:fixed focus:top-4 focus:left-4 focus:z-[9999] focus:px-4 focus:py-2 focus:bg-rose-600 focus:text-white focus:rounded-xl focus:font-semibold focus:outline-none"
				>
					Skip to content
				</a>
				<div className="grain-overlay" aria-hidden />
				<div id="main-content">{children}</div>
				<ToastAlert />
			</body>
		</html>
	);
}
