import { Suspense } from 'react';
import { AuthPage } from '@/features/auth/auth-page';
export default function Page() {
  return (
    <Suspense fallback={<div className="container page">Загрузка…</div>}>
      <AuthPage view="login" />
    </Suspense>
  );
}
