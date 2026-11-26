'use client';
import { DEMO } from '@/lib/state/store';
import { DemoAccountPage } from './demo-account-page';
import { RealAccountPage } from './real-account-page';
export type AccountView =
  | 'dashboard'
  | 'applications'
  | 'saved'
  | 'contacts'
  | 'profile'
  | 'edit'
  | 'portfolio'
  | 'settings';
export function AccountPage({ view }: { view: AccountView }) {
  return DEMO ? <DemoAccountPage view={view} /> : <RealAccountPage view={view} />;
}
