'use client';
import { useSyncExternalStore } from 'react';
import { migrateDemoState } from './migrate';
import { initialState } from '@/data/demo';
import { ApiError, apiRequest } from '@/lib/api/client';
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
let sessionError: string | undefined;
let selectedCompanyId: string | null = null;
let selectedForUser: string | null = null;
const COMPANY_KEY = 'firstorder:company';
function rememberCompany(userId: string, companyId: string | null) {
  try {
    if (companyId) sessionStorage.setItem(COMPANY_KEY, JSON.stringify({ userId, companyId }));
    else sessionStorage.removeItem(COMPANY_KEY);
  } catch {
    /* Selection still works in memory when storage is unavailable. */
  }
}
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
  let loadError: string | undefined;
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
            next.portfolio = items(value).map((item) => ({
              ...item,
              description: item.description || '',
              url: item.url || '',
            }));
          })
          .catch(() => {
            next.portfolio = [];
          }),
      );
    }
    const memberships = profile.companies || [];
    let chosen = selectedForUser === profile.id ? selectedCompanyId : null;
    if (!chosen) {
      try {
        const saved = JSON.parse(sessionStorage.getItem(COMPANY_KEY) || 'null');
        if (saved?.userId === profile.id) chosen = saved.companyId;
      } catch {
        /* Ignore malformed or unavailable preference storage. */
      }
    }
    const companyId = memberships.some((item) => item.companyId === chosen)
      ? chosen
      : memberships[0]?.companyId || null;
    if (version === generation && !signal.aborted) {
      selectedCompanyId = companyId;
      selectedForUser = profile.id;
      rememberCompany(profile.id, companyId);
    }
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
  } catch (error) {
    if (!signal.aborted && !(error instanceof ApiError && error.status === 401)) {
      loadError = error instanceof Error ? error.message : 'Не удалось загрузить аккаунт.';
    }
  }
  await orderTask;
  if (version !== generation || signal.aborted) return latestRefresh || state;
  state = next;
  sessionError = loadError;
  if (!next.signedIn && !loadError) {
    selectedCompanyId = null;
    selectedForUser = null;
    rememberCompany('', null);
  }
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

/** A connection failure must not be presented as a successful sign-out. */
export function useSessionError() {
  return useSyncExternalStore(
    subscribe,
    () => sessionError,
    () => undefined,
  );
}
export function useSelectedCompanyId() {
  return useSyncExternalStore(
    subscribe,
    () => (DEMO ? state.company.id || null : selectedCompanyId),
    () => null,
  );
}
export async function selectCompany(id: string): Promise<AppState> {
  if (DEMO) {
    if (id !== state.company.id) throw new Error('Компания недоступна.');
    return state;
  }
  if (!state.signedIn || !state.profile.companies?.some((company) => company.companyId === id)) {
    throw new Error('Нет доступа к этой компании.');
  }
  if (id === selectedCompanyId && state.company.id === id) return state;
  selectedCompanyId = id;
  selectedForUser = state.profile.id;
  rememberCompany(state.profile.id, id);
  state = { ...state, company: structuredClone(emptyState.company) };
  notify();
  return refreshAppState();
}
