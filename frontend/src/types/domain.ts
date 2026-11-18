export type Role = 'developer' | 'company' | 'moderator';
export type WebsiteStatus = 'not_found' | 'found' | 'needs_review';
export interface Company { id: string; name: string; city: string; category: string; address: string; description: string; websiteStatus: WebsiteStatus; checkedAt: string; website?: string; phone?: string; email?: string; }
export interface Order { id: string; companyId: string; companyName: string; city: string; title: string; description: string; budgetMin: number; budgetMax: number; deadline: string; status: 'open' | 'in_progress' | 'completed' | 'draft'; applicationsCount: number; publishedAt: string; }
export interface Profile { id: string; name: string; email: string; role: Role; city: string; bio: string; skills: string[]; }
export interface Paged<T> { items: T[]; total: number; page: number; pageSize: number; }
