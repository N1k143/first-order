'use client';
import { useSyncExternalStore } from 'react';
import { initialState } from '@/data/demo';
import type { AppState } from '@/types/domain';
export const DEMO = process.env.NEXT_PUBLIC_DATA_MODE !== 'api';
const KEY = 'firstorder:demo:v1';
const emptyState: AppState = {
  ...initialState,
  signedIn: false,
  profile: {
    id: '',
    name: '',
    email: '',
    role: 'developer',
    city: '',
    bio: '',
    headline: '',
    skills: [],
    github: '',
  },
  company: {
    id: '',
    name: '',
    initials: '',
    city: '',
    category: '',
    address: '',
    description: '',
    websiteStatus: 'needs_review',
    checkedAt: '',
  },
  favorites: [],
  portfolio: [],
  contacts: [],
  applications: [],
  orders: [],
  members: [],
  corrections: [],
  hiddenCompanies: [],
  history: [],
};
const serverState = DEMO ? initialState : emptyState;
let state = serverState;
let initialized = false;
const listeners = new Set<() => void>();
function notify() {
  listeners.forEach((l) => l());
}
function load() {
  if (initialized || typeof window === 'undefined') return;
  initialized = true;
  if (DEMO) {
    try {
      const value = JSON.parse(localStorage.getItem(KEY) || 'null');
      if (
        value &&
        value.profile &&
        Array.isArray(value.orders) &&
        Array.isArray(value.applications)
      )
        state = { ...initialState, ...value };
    } catch {
      /* Unavailable storage or outdated demo state: start with fixtures. */
    }
  }
  queueMicrotask(notify);
}
function subscribe(listener: () => void) {
  listeners.add(listener);
  load();
  return () => {
    listeners.delete(listener);
  };
}
export function useAppState() {
  return useSyncExternalStore(
    subscribe,
    () => state,
    () => serverState,
  );
}
export function updateDemo(update: (current: AppState) => AppState) {
  if (!DEMO) throw new Error('Local mutations are disabled in API mode');
  state = update(state);
  try {
    localStorage.setItem(KEY, JSON.stringify(state));
  } catch {
    /* In-memory interaction still works in private browsing. */
  }
  notify();
}
export function resetDemo() {
  updateDemo(() => structuredClone(initialState));
}
