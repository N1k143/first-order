'use client';

import Image from 'next/image';
import { useState } from 'react';

export function Avatar({ name, src, size = 44 }: { name: string; src?: string; size?: number }) {
  const [failedSource, setFailedSource] = useState<string>();
  const initials =
    name
      .trim()
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => Array.from(part)[0])
      .join('')
      .toUpperCase() || 'П';
  const allowed =
    src && (/^https?:\/\//i.test(src) || /^data:image\/(png|jpeg|webp);base64,/.test(src));
  return (
    <span className="avatar" style={{ width: size, height: size }}>
      {allowed && failedSource !== src ? (
        <Image
          src={src}
          alt=""
          width={size}
          height={size}
          unoptimized
          onError={() => setFailedSource(src)}
        />
      ) : (
        <span aria-hidden="true">{initials}</span>
      )}
    </span>
  );
}
