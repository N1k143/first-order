import { Suspense } from 'react';
import { AuthPage } from '@/features/auth/auth-page';

export default function Page() {
  return (
    <Suspense>
      <AuthPage view="login" entryRole="developer" />
    </Suspense>
  );
}
