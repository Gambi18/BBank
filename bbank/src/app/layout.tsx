import type { Metadata } from "next";
import { Geist, Geist_Mono, Instrument_Serif } from "next/font/google";
import "./globals.css";
import ToastAlert from "@/components/ToastAlert";

const geistSans = Geist({
	variable: "--font-geist-sans",
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
};

export default function RootLayout({
	children,
}: Readonly<{
	children: React.ReactNode;
}>) {
	return (
		<html lang="en">
			<body
				className={`${geistSans.variable} ${geistMono.variable} ${instrumentSerif.variable} antialiased`}
			>
				{children}
				<ToastAlert />
			</body>
		</html>
	);
}
