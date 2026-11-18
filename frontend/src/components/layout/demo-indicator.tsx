'use client';
import Link from 'next/link';
import { DEMO } from '@/lib/state/store';
export function DemoIndicator() {
  return DEMO ? (
    <div className="demo-indicator">
      Деморежим · Данные сохраняются в этом браузере. <Link href="/settings">Настройки →</Link>
    </div>
  ) : null;
}
