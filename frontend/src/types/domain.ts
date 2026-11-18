import type { ModerationActionRow } from './database';

export type Role = 'developer' | 'company' | 'moderator';
export type WebsiteStatus = 'not_found' | 'found' | 'needs_review';
export interface Company {
  id: string;
  name: string;
  initials: string;
  city: string;
  category: string;
  categoryId?: string | null;
  sourceUrl?: string | null;
  visibility?: string;
  address: string;
  websiteStatus: WebsiteStatus;
  checkedAt: string | null;
  website?: string;
  phone?: string;
  email?: string;
}
export interface Order {
  id: string;
  companyId: string;
  companyName: string;
  city: string;
  title: string;
  description: string;
  budgetMin: number | null;
  budgetMax: number | null;
  deadline: string | null;
  status: 'open' | 'in_progress' | 'completed' | 'draft';
  applicationsCount: number;
  publishedAt: string;
  selectedApplicationId?: string | null;
  visibility?: boolean;
}
export interface Profile {
  /** Stored URL; data URLs are allowed only in the local demo upload flow. */
  avatarUrl?: string;
  id: string;
  name: string;
  email: string;
  role: Role;
  city: string;
  bio: string;
  headline: string;
  skills: string[];
  github: string;
}
export interface Portfolio {
  id: string;
  title: string;
  description: string;
  url: string;
}
export interface Application {
  id: string;
  orderId: string;
  name: string;
  profileId: string;
  price: number | null;
  deadline: string | null;
  letter: string;
  status: 'pending' | 'selected' | 'rejected' | 'withdrawn' | 'completed';
}
export interface Contact {
  companyId: string;
  status: string;
  note: string;
  date: string;
  contactedAt?: string | null;
}
export interface Member {
  id: string;
  name: string;
  email: string;
  role: 'owner' | 'member';
}
export interface Correction {
  id: string;
  companyId: string;
  message: string;
  status: 'pending' | 'approved' | 'rejected';
  reason?: string;
  requestedBy?: string;
  resolvedBy?: string | null;
  resolvedAt?: string | null;
}
export interface AppState {
  profile: Profile;
  signedIn: boolean;
  favorites: string[];
  portfolio: Portfolio[];
  contacts: Contact[];
  applications: Application[];
  orders: Order[];
  company: Company;
  members: Member[];
  corrections: Correction[];
  hiddenCompanies: string[];
  history: ModerationActionRow[];
}
export interface Paged<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}
