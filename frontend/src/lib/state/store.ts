'use client';
import { useSyncExternalStore } from 'react';
import { migrateDemoState } from './migrate';
import { initialState } from '@/data/demo';
import { apiRequest } from '@/lib/api/client';
import type {
  AppState,
  Application,
  Company,
  Contact,
  Order,
  Portfolio,
  Profile,
} from '@/types/domain';

export const DEMO = process.env.NEXT_PUBLIC_DATA_MODE !== 'api';
const KEY = 'firstorder:demo:v1';
const emptyState: AppState = {
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
    companies: [],
  },
  signedIn: false,
  favorites: [],
  portfolio: [],
  contacts: [],
  applications: [],
  orders: [],
  members: [],
  corrections: [],
  hiddenCompanies: [],
  history: [],
  company: {
    id: '',
    name: '',
    initials: '',
    city: '',
    category: '',
    address: '',
    description: '',
    websiteStatus: 'needs_review',
    checkedAt: null,
  },
};
const serverSnapshot = DEMO ? initialState : emptyState;
let state = serverSnapshot;
let initialized = false;
let ready = false;
let generation = 0;
let loadingController: AbortController | undefined;
let latestRefresh: Promise<AppState> | undefined;
const listeners = new Set<() => void>();
function notify() {
  listeners.forEach((listener) => listener());
}
function items<T>(value: T[] | { items: T[] }): T[] {
  return Array.isArray(value) ? value : value.items;
}

/** Replace API state only with server data; failed requests never use fixtures. */
export function refreshAppState(): Promise<AppState> {
  if (DEMO || typeof window === 'undefined') return Promise.resolve(state);
  const version = ++generation;
  loadingController?.abort();
  const controller = new AbortController();
  loadingController = controller;
  latestRefresh = loadRemoteState(version, controller.signal);
  return latestRefresh;
}
async function loadRemoteState(version: number, signal: AbortSignal): Promise<AppState> {
  const next: AppState = structuredClone(emptyState);
  const orderTask = apiRequest<Order[] | { items: Order[] }>('/orders?limit=100', { signal })
    .then((value) => {
      next.orders = items(value);
    })
    .catch(() => {
      next.orders = [];
    });
  try {
    const profile = await apiRequest<Profile>('/me', { signal });
    next.profile = {
      ...emptyState.profile,
      ...profile,
      avatarUrl: profile.avatarUrl || '',
      companies: profile.companies || [],
      skills: profile.skills || [],
      bio: profile.bio || '',
      github: profile.github || '',
    };
    next.signedIn = true;
    const tasks: Promise<void>[] = [];
    if (profile.role !== 'moderator') {
      tasks.push(
        apiRequest<Company[] | { items: Company[] }>('/me/favorites?limit=100', { signal })
          .then((value) => {
            next.favorites = items(value).map((company) => company.id);
          })
          .catch(() => {
            next.favorites = [];
          }),
      );
      tasks.push(
        apiRequest<Contact[] | { items: Contact[] }>('/me/contacts?limit=100', { signal })
          .then((value) => {
            next.contacts = items(value).map((contact) => ({
              ...contact,
              note: contact.note || '',
              date: contact.date || '',
              status:
                (
                  {
                    planned: 'Планирую написать',
                    contacted: 'Написал',
                    replied: 'Ответили',
                    declined: 'Не актуально',
                  } as Record<string, string>
                )[contact.status] || contact.status,
            }));
          })
          .catch(() => {
            next.contacts = [];
          }),
      );
      tasks.push(
        apiRequest<Application[] | { items: Application[] }>('/me/applications?limit=100', {
          signal,
        })
          .then((value) => {
            next.applications = items(value);
          })
          .catch(() => {
            next.applications = [];
          }),
      );
      tasks.push(
        apiRequest<Portfolio[] | { items: Portfolio[] }>('/me/portfolio?limit=100', { signal })
          .then((value) => {
            next.portfolio = items(value);
          })
          .catch(() => {
            next.portfolio = [];
          }),
      );
    }
    const companyId = profile.companies?.[0]?.companyId;
    if (companyId)
      tasks.push(
        apiRequest<Company>(`/companies/${encodeURIComponent(companyId)}`, { signal })
          .then((value) => {
            next.company = value;
          })
          .catch(() => {
            next.company = structuredClone(emptyState.company);
          }),
      );
    await Promise.all(tasks);
  } catch {
    // Clear personal data for signed-out/unavailable sessions. Page resources report
    // request errors separately. A newer refresh owns publication after cancellation.
  }
  await orderTask;
  if (version !== generation || signal.aborted) return latestRefresh || state;
  state = next;
  ready = true;
  notify();
  return state;
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
        state = migrateDemoState({ ...initialState, ...value });
    } catch {
      /* Unavailable demo storage: use fixtures. */
    }
    ready = true;
    queueMicrotask(notify);
  } else {
    void refreshAppState();
  }
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
    () => serverSnapshot,
  );
}
export function updateDemo(update: (current: AppState) => AppState) {
  if (!DEMO) throw new Error('Local mutations are disabled in API mode');
  state = update(state);
  try {
    localStorage.setItem(KEY, JSON.stringify(state));
  } catch {
    /* In-memory demo remains available. */
  }
  notify();
}
export function resetDemo() {
  updateDemo(() => structuredClone(initialState));
}
/** Keep editable forms behind the initial persisted-demo or remote-profile load. */
export function useStoreReady() {
  return useSyncExternalStore(
    subscribe,
    () => ready,
    () => false,
  );
}
