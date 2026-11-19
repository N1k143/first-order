'use client';

import { AccountPage } from '@/features/account/account-page';
import { MyApplicationsPage } from '@/features/workspace/orders-workspace';
import { DEMO } from '@/lib/state/store';

export default function Page() {
  return DEMO ? <AccountPage view="applications" /> : <MyApplicationsPage />;
}
