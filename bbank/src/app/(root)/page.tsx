import Image from "next/image";
import Link from "next/link";
import { redirect } from "next/navigation";
import {
   FaDroplet, FaHeartPulse, FaUsers, FaArrowRight, FaCalendarCheck,
   FaUserPlus, FaHandHoldingHeart, FaPhone, FaEnvelope, FaLocationDot,
} from "react-icons/fa6";
import Reveal from "@/components/Reveal";

const bloodTypes = ["A+", "A−", "B+", "B−", "AB+", "AB−", "O+", "O−"];

const stats = [
   { icon: FaUsers, value: "10,000+", label: "Registered donors" },
   { icon: FaDroplet, value: "25,000+", label: "Units collected" },
   { icon: FaHeartPulse, value: "50,000+", label: "Lives impacted" },
];

const steps = [
   {
      icon: FaUserPlus,
      title: "Create your profile",
      body: "Sign up in under a minute. Tell us your blood type and where you are — that's all we need to match you.",
   },
   {
      icon: FaCalendarCheck,
      title: "Request an appointment",
      body: "When you're ready to give, request a slot. Our coordinators confirm a date that works for you.",
   },
   {
      icon: FaHandHoldingHeart,
      title: "Donate & save lives",
      body: "Show up, donate, and walk out a hero. One donation can save up to three lives.",
   },
];

export default function Home() {
   async function submitContact() {
      'use server'
      // Placeholder: in a real deployment this would email/store the message.
      redirect('/?success=Message+sent!+We+will+get+back+to+you.#contact')
   }

   return (
      <main>
         {/* ============ Hero ============ */}
         <section className="relative mesh overflow-hidden">
            <div className="absolute inset-0 grid-lines pointer-events-none" aria-hidden />
            {/* Abstract blobs */}
            <div className="blob w-96 h-96 bg-rose-100/80 -top-24 -right-24" aria-hidden />
            <div className="blob w-72 h-72 bg-amber-50 top-1/2 -left-24" aria-hidden />
            <div className="relative mx-auto max-w-6xl px-6 pt-36 pb-20 lg:pt-44 lg:pb-28 grid lg:grid-cols-2 gap-16 items-center">
               <div>
                  <div className="eyebrow animate-fade-up">
                     <span className="w-1.5 h-1.5 rounded-full bg-rose-600 live-dot" />
                     Every drop counts
                  </div>
                  <h1 className="headline text-5xl md:text-6xl lg:text-7xl mt-6 animate-fade-up anim-delay-1">
                     Give blood.{" "}
                     <span className="display-serif text-gradient">Give someone</span>
                     <br />
                     <span className="display-serif text-gradient">tomorrow.</span>
                  </h1>
                  <p className="text-zinc-600 text-lg mt-6 max-w-md animate-fade-up anim-delay-2">
                     BloodBank connects willing donors with hospitals in critical need —
                     a modern platform for an act as old as kindness itself.
                  </p>
                  <div className="flex flex-wrap gap-4 mt-9 animate-fade-up anim-delay-3">
                     <Link href="/signup" className="btn btn-primary btn-lg">
                        Become a donor <FaArrowRight className="text-sm" />
                     </Link>
                     <Link href="/#how" className="btn btn-ghost btn-lg">How it works</Link>
                  </div>
                  <div className="flex items-center gap-3 mt-10 text-sm text-zinc-500 animate-fade-up anim-delay-4">
                     <div className="flex -space-x-2.5">
                        {["KM", "AT", "JN", "SE"].map((n) => (
                           <span key={n} className="avatar !w-8 !h-8 ring-2 ring-[#fafaf9]">{n}</span>
                        ))}
                     </div>
                     Trusted by thousands of donors across the region
                  </div>
               </div>

               <div className="relative animate-scale-in anim-delay-2">
                  <div className="card overflow-hidden p-2 animate-float">
                     <Image
                        src="/bd.jpg"
                        priority
                        alt="A donor giving blood"
                        width={640}
                        height={640}
                        className="rounded-xl object-cover w-full"
                     />
                  </div>
                  {/* Floating chips */}
                  <div className="absolute -left-4 top-10 blur-panel rounded-2xl px-4 py-3 flex items-center gap-3 shadow-lg shadow-black/5 animate-float" style={{ animationDelay: '0.8s' }}>
                     <span className="w-9 h-9 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center"><FaDroplet /></span>
                     <div>
                        <div className="text-sm font-semibold">O− needed</div>
                        <div className="text-xs text-zinc-500">Universal donor</div>
                     </div>
                  </div>
                  <div className="absolute -right-3 bottom-12 blur-panel rounded-2xl px-4 py-3 flex items-center gap-3 shadow-lg shadow-black/5 animate-float" style={{ animationDelay: '1.6s' }}>
                     <span className="w-9 h-9 rounded-xl bg-emerald-50 text-emerald-600 flex items-center justify-center"><FaCalendarCheck /></span>
                     <div>
                        <div className="text-sm font-semibold">Appointment confirmed</div>
                        <div className="text-xs text-zinc-500">Tomorrow, 9:00 AM</div>
                     </div>
                  </div>
               </div>
            </div>
         </section>

         {/* ============ Blood types marquee ============ */}
         <section className="border-y border-black/5 bg-white py-5 overflow-hidden" aria-hidden>
            <div className="marquee">
               {[...bloodTypes, ...bloodTypes, ...bloodTypes, ...bloodTypes].map((t, i) => (
                  <span key={i} className="flex items-center gap-3 text-zinc-300 font-semibold text-lg">
                     <FaDroplet className="text-rose-200" /> {t}
                  </span>
               ))}
            </div>
         </section>

         {/* ============ Stats ============ */}
         <section className="mx-auto max-w-6xl px-6 py-20">
            <div className="grid sm:grid-cols-3 gap-5">
               {stats.map((s, i) => (
                  <Reveal key={s.label} delay={i * 0.1}>
                     <div className="card card-hover card-spot p-7">
                        <s.icon className="text-rose-600 text-xl" />
                        <div className="text-3xl font-bold tracking-tight mt-4">{s.value}</div>
                        <div className="text-zinc-500 text-sm mt-1">{s.label}</div>
                     </div>
                  </Reveal>
               ))}
            </div>
         </section>

         {/* ============ How it works ============ */}
         <section id="how" className="mx-auto max-w-6xl px-6 py-20 scroll-mt-24">
            <Reveal>
               <div className="eyebrow">The process</div>
               <h2 className="headline text-4xl md:text-5xl mt-4">
                  Three steps to <span className="display-serif text-gradient">saving a life</span>
               </h2>
            </Reveal>
            <div className="grid md:grid-cols-3 gap-5 mt-12">
               {steps.map((step, i) => (
                  <Reveal key={step.title} delay={i * 0.12}>
                     <div className="card card-hover card-spot p-7 h-full">
                        <div className="flex items-center justify-between">
                           <span className="w-11 h-11 rounded-xl bg-rose-50 text-rose-600 flex items-center justify-center text-lg">
                              <step.icon />
                           </span>
                           <span className="text-5xl font-bold text-black/5">0{i + 1}</span>
                        </div>
                        <h3 className="font-semibold text-lg mt-5">{step.title}</h3>
                        <p className="text-zinc-500 text-sm mt-2 leading-relaxed">{step.body}</p>
                     </div>
                  </Reveal>
               ))}
            </div>
         </section>

         {/* ============ About ============ */}
         <section id="about" className="border-y border-black/5 bg-white scroll-mt-24 relative overflow-hidden">
            <div className="blob w-80 h-80 bg-rose-50 -bottom-32 -right-20" aria-hidden />
            <div className="relative mx-auto max-w-6xl px-6 py-24 grid lg:grid-cols-2 gap-16 items-center">
               <Reveal>
                  <div className="card overflow-hidden p-2">
                     <Image
                        src="/pexels-charliehelen-robinson-4531306.jpg"
                        alt="Blood donation in progress"
                        width={600}
                        height={600}
                        className="rounded-xl object-cover w-full"
                     />
                  </div>
               </Reveal>
               <Reveal delay={0.15}>
                  <div className="eyebrow">About us</div>
                  <h2 className="headline text-4xl md:text-5xl mt-4">
                     A central portal for <span className="display-serif text-gradient">those who give</span>
                  </h2>
                  <p className="text-zinc-600 mt-6 leading-relaxed">
                     The Blood Bank was created as a platform for those who can donate. It provides
                     information about what donating blood entails and tracks everyone who registers
                     to become a donor — from first signup to the moment they give.
                  </p>
                  <p className="text-zinc-600 mt-4 leading-relaxed">
                     It equally serves as a bridge between donors and local hospitals in critical
                     need of blood, keeping both sides connected and informed.
                  </p>
                  <Link href="/signup" className="btn btn-ghost mt-8">
                     Join the registry <FaArrowRight className="text-sm" />
                  </Link>
               </Reveal>
            </div>
         </section>

         {/* ============ Contact ============ */}
         <section id="contact" className="mx-auto max-w-6xl px-6 py-24 scroll-mt-24">
            <Reveal>
               <div className="eyebrow">Contact</div>
               <h2 className="headline text-4xl md:text-5xl mt-4">
                  Get in <span className="display-serif text-gradient">touch</span>
               </h2>
            </Reveal>
            <div className="grid lg:grid-cols-5 gap-5 mt-12">
               <Reveal className="lg:col-span-3">
                  <div className="card p-8">
                     <form action={submitContact} className="flex flex-col gap-5">
                        <div className="grid sm:grid-cols-2 gap-5">
                           <div>
                              <label className="label" htmlFor="c-name">Full name</label>
                              <input id="c-name" type="text" name="name" placeholder="Jane Doe" className="field" required />
                           </div>
                           <div>
                              <label className="label" htmlFor="c-email">Email</label>
                              <input id="c-email" type="email" name="email" placeholder="jane@example.com" className="field" required />
                           </div>
                        </div>
                        <div>
                           <label className="label" htmlFor="c-msg">Message</label>
                           <textarea id="c-msg" name="message" placeholder="How can we help?" rows={5} className="field resize-none" required />
                        </div>
                        <button type="submit" className="btn btn-primary self-end px-8">Send message</button>
                     </form>
                  </div>
               </Reveal>
               <Reveal delay={0.15} className="lg:col-span-2">
                  <div className="card p-8 h-full flex flex-col gap-6">
                     <div>
                        <h3 className="font-semibold text-lg">Reach us directly</h3>
                        <p className="text-zinc-500 text-sm mt-1">We typically respond within a day.</p>
                     </div>
                     <ul className="flex flex-col gap-4 text-sm">
                        <li className="flex items-center gap-3 text-zinc-600">
                           <span className="w-9 h-9 rounded-xl bg-zinc-100 flex items-center justify-center text-rose-600"><FaPhone /></span>
                           <a href="tel:+237653532929" className="hover:text-zinc-900 transition-colors">+237 6 53 53 29 29</a>
                        </li>
                        <li className="flex items-center gap-3 text-zinc-600">
                           <span className="w-9 h-9 rounded-xl bg-zinc-100 flex items-center justify-center text-rose-600"><FaEnvelope /></span>
                           <a href="mailto:hello@bloodbank.example" className="hover:text-zinc-900 transition-colors">hello@bloodbank.example</a>
                        </li>
                        <li className="flex items-center gap-3 text-zinc-600">
                           <span className="w-9 h-9 rounded-xl bg-zinc-100 flex items-center justify-center text-rose-600"><FaLocationDot /></span>
                           Douala, Cameroon
                        </li>
                     </ul>
                     <div className="mt-auto card !bg-rose-50/60 !border-rose-200 p-5">
                        <div className="font-semibold text-rose-700 flex items-center gap-2"><FaDroplet /> Urgent need?</div>
                        <p className="text-zinc-600 text-sm mt-1.5">
                           Hospitals with critical shortages can call our 24/7 line for priority matching.
                        </p>
                     </div>
                  </div>
               </Reveal>
            </div>
         </section>

         {/* ============ Final CTA ============ */}
         <section className="relative mesh border-t border-black/5 overflow-hidden">
            <div className="blob w-80 h-80 bg-rose-100/70 -top-24 left-1/2 -translate-x-1/2" aria-hidden />
            <div className="relative mx-auto max-w-3xl px-6 py-24 text-center">
               <Reveal>
                  <h2 className="headline text-4xl md:text-6xl">
                     Ready to be <span className="display-serif text-gradient">someone&apos;s miracle?</span>
                  </h2>
                  <p className="text-zinc-600 mt-5 max-w-lg mx-auto">
                     It takes ten minutes of your day and costs nothing.
                     For someone out there, it&apos;s everything.
                  </p>
                  <Link href="/signup" className="btn btn-primary btn-lg mt-9 pulse-ring">
                     Become a donor today <FaArrowRight className="text-sm" />
                  </Link>
               </Reveal>
            </div>
         </section>
      </main>
   );
}
