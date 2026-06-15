"use client";

import { useState } from "react";

// Company mark via Google's favicon service (free, no key — Clearbit's logo API
// is dead). Rendered on a light tile so dark favicons stay legible; falls back
// to a blue monogram.
export function Logo({ domain, name, size = 36 }: { domain: string; name: string; size?: number }) {
  const [failed, setFailed] = useState(false);

  if (!domain || failed) {
    return (
      <div
        className="flex shrink-0 items-center justify-center rounded-md bg-raised font-mono font-medium text-accent"
        style={{ width: size, height: size, fontSize: size * 0.4 }}
      >
        {name.charAt(0)}
      </div>
    );
  }
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={`https://www.google.com/s2/favicons?domain=${domain}&sz=128`}
      width={size}
      height={size}
      alt={`${name} icon`}
      onError={() => setFailed(true)}
      className="shrink-0 rounded-md bg-white/95 object-contain p-1.5"
    />
  );
}
