import "./globals.css";
import Link from "next/link";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "ReqRadar",
  description: "Watchlist-first hiring intelligence",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-gray-50 text-gray-900 antialiased">
        <header className="border-b border-gray-200 bg-white">
          <nav className="mx-auto flex h-14 max-w-6xl items-center gap-6 px-4">
            <Link href="/" className="font-bold text-indigo-600">
              ReqRadar
            </Link>
            <Link href="/" className="text-sm text-gray-600 hover:text-gray-900">
              Watchlist
            </Link>
            <Link href="/firehose" className="text-sm text-gray-600 hover:text-gray-900">
              Firehose
            </Link>
          </nav>
        </header>
        <main className="mx-auto max-w-6xl px-4 py-6">{children}</main>
      </body>
    </html>
  );
}
