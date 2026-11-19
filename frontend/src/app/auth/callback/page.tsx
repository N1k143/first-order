import { Suspense } from 'react';
import { AuthCallbackPage } from '@/features/auth/auth-page';
export default function Page() {
  return (
    <Suspense fallback={<div className="container page">Подтверждение аккаунта…</div>}>
      <AuthCallbackPage />
    </Suspense>
  );
}
