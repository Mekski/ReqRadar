"use client";

import { useState } from "react";

// Company logo via Clearbit (we already store the domain). Falls back to a
// letter avatar if there's no domain or the image fails.
export function Logo({ domain, name, size = 40 }: { domain: string; name: string; size?: number }) {
  const [failed, setFailed] = useState(false);

  if (!domain || failed) {
    return (
      <div
        className="flex items-center justify-center rounded bg-gray-200 font-semibold text-gray-500"
        style={{ width: size, height: size, fontSize: size * 0.45 }}
      >
        {name.charAt(0)}
      </div>
    );
  }
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={`https://logo.clearbit.com/${domain}`}
      width={size}
      height={size}
      alt={`${name} logo`}
      className="rounded bg-white object-contain"
      onError={() => setFailed(true)}
    />
  );
}
