"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { BeeMark } from "@/components/BeeMark";
import { REPO_URL } from "@/lib/api";

/** Top navigation for the website. The Playground has its own sidebar, so it hides this. */
export function SiteHeader() {
  const path = usePathname();
  if (path?.startsWith("/playground") || path?.startsWith("/monitor")) return null;
  return (
    <header className="wrap nav">
      <Link href="/" className="brand" aria-label="Beeline home">
        <BeeMark size={30} />
        Beeline
      </Link>
      <nav className="nav-links" aria-label="Main">
        <Link href="/playground">Playground</Link>
        <Link href="/vendor">Vendors</Link>
        <a href={REPO_URL} target="_blank" rel="noreferrer">
          GitHub
        </a>
      </nav>
    </header>
  );
}
