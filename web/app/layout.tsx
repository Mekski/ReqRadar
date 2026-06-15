import "./globals.css";
import Link from "next/link";
import type { Metadata } from "next";
import { Geist, JetBrains_Mono } from "next/font/google";

const sans = Geist({ subsets: ["latin"], variable: "--font-geist" });
const mono = JetBrains_Mono({ subsets: ["latin"], weight: ["400", "500", "700"], variable: "--font-jbmono" });

export const metadata: Metadata = {
  title: "ReqRadar",
  description: "Watchlist-first hiring intelligence",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`}>
      <body className="min-h-screen antialiased">
        <header className="sticky top-0 z-20 border-b border-line bg-bg/80 backdrop-blur-md">
          <nav className="mx-auto flex h-16 max-w-6xl items-center gap-8 px-6">
            <Link href="/" className="flex items-center gap-2 font-mono text-sm font-bold tracking-tight text-ink">
              <span className="pulse-soft inline-block h-1.5 w-1.5 rounded-full bg-accent" />
              req<span className="text-accent">radar</span>
            </Link>
            <div className="flex items-center gap-6 font-mono text-[13px]">
              <NavLink href="/">watchlist</NavLink>
              <NavLink href="/firehose">firehose</NavLink>
            </div>
          </nav>
        </header>
        <main className="mx-auto max-w-6xl px-6 py-10">{children}</main>
      </body>
    </html>
  );
}

function NavLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link href={href} className="text-muted transition-colors hover:text-accent">
      {children}
    </Link>
  );
}
