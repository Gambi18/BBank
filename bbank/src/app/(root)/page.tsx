import Image from "next/image";
import Link from "next/link";
import { FaInstagram, FaFacebook, FaTwitter, FaLinkedin, FaPhone } from "react-icons/fa";

export default function Home() {
   return (
      <main className="">

         {/* This is the Hero section */}
         <div className="hero flex flex-col lg:flex-row p-5 lg:h-[calc(100vh-5rem)] items-center text-black">
            <div className="hero-left w-full lg:w-1/2 text-center lg:text-left m-auto font-semibold p-20">
               <div className="text-3xl font-extrabold">Welcome to THE</div>
               <h1 className="text-4xl md:text-6xl lg:text-8xl font-extrabold m-0"><span className="text-red-700">BLOOD</span>BANK</h1>
               <p className="font-light w-max lg:w-full text-center mx-auto md:text-left lg:m-0">Donate  Blood, Save  Lives.  You  can  make  a difference .</p>
               <div className="mt-10">
                  <button className="bg-transparent hover:bg-red-700 rounded-2xl border-2 border-red-700 px-10 py-5 me-2 my-5 font text-red-700 hover:text-gray-200 font-bold text-xl">Learn More</button>
                  <Link href="/signup"><button className="bg-red-700 hover:bg-red-800 rounded-2xl px-10 py-5 my-5 border-2 border-red-700 text-gray-200 font-extrabold text-xl">Sign Up</button></Link>
               </div>
            </div>
            <div className="hero-right p-5 w-1/2 mx-auto">
               <Image src="/bd.jpg" className="rounded-2xl" alt="" width={700} height={700} />
            </div>
         </div>

         {/* This is the About section */}
         <div id="about" className="flex flex-col lg:flex-row p-5 bg-linear-180 lg:h-screen items-center bg-slate-900 text-white">
            <div className="about-left w-full lg:w-1/2 m-auto font-semibold">
               <Image src="/pexels-charliehelen-robinson-4531306.jpg" className="rounded-2xl mx-auto" alt="" width={600} height={600} />
            </div>
            <div className="about-right p-30 w-full lg:w-1/2 mx-auto">
               <h1 className="text-6xl font-semibold border-b-3 border-red-700 w-max">About Us</h1>
               <p className="mt-15">The Blood Bank was created as a platform for those who can donate. This website acts as a central portal which provides information about what donating blood entails and tracking the number of people who register to become donors and eventually donate.</p>
               <p className="mt-10">Our platform equally serves as a means for donors to connect and stay updated with local hospitals which may be in critical need for blood or simply as an means to view blood requests from hospitals in need.</p>
            </div>
         </div>

         {/* This is the Contact section */}
         <div id="contact" className="p-5 bg-linear-180 items-center text-center  bg-slate-300 text-black">
            <h1 className="text-6xl font-semibold border-b-3 border-red-700 w-max justify-self-center">Get In Touch</h1>
            <div className="flex flex-col lg:flex-row mx-auto p-5 justify-center">
               <div className="w-full lg:w-1/2">
                  <div className="card bg-slate-900 text-white rounded-2xl m-5 p-5">
                     <h2 className="text-xl font-semibold card-title text-center border-b border-red-700 w-max justify-self-center">Your Info</h2>
                     <form action="submit" className="p-5 text-[1.1rem] flex flex-col">
                        <input type="text" placeholder="Full Name" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" />
                        <input type="email" placeholder="Email Address" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" />
                        <textarea placeholder="Message" className="bg-white text-black placeholder:text-gray-300 rounded-md py-1 px-2 w-full my-5" />
                        <button type="submit" className="bg-red-700 hover:bg-red-800 rounded-md px-10 py-2 my-5 border-2 border-red-700 text-gray-200 font-extrabold text-xl w-min self-end">Submit</button>
                     </form>
                  </div>
               </div>
               <div className="items-center align-middle">
                  <div className="card bg-slate-900 text-white rounded-2xl p-5 m-5 ">
                     <h2 className="text-xl font-semibold card-title text-center border-b border-red-700 w-max justify-self-center">Our info</h2>
                     <ul className="socials">
                        <li className=" p-3"><a href="#" className="flex justify-center"><FaInstagram className="m-2" />@the-bloodbank_ig</a></li>
                        <li className=" p-3"><a href="#" className="flex justify-center"><FaFacebook className="m-2" />@the-bloodbank_fb</a></li>
                        <li className=" p-3"><a href="#" className="flex justify-center"><FaTwitter className="m-2" />@the-bloodbank_x</a></li>
                        <li className=" p-3"><a href="#" className="flex justify-center"><FaLinkedin className="m-2" />@the-bloodbank_li</a></li>
                        <li className=" p-3"><a href="#" className="flex justify-center"><FaPhone className="m-2" />+237 6 53 53 29 29</a></li>
                     </ul>
                  </div>
               </div>
            </div>
         </div>
      </main>
   );
}
