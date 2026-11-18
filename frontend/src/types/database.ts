/** Schema supplied by the project owner. These are row shapes, not API endpoints. */
export interface ProfileRow {
  readonly user_id: string;
  readonly role: string;
  name: string;
  avatar_url: string | null;
  bio: string | null;
  skills: string | null;
  github_url: string | null;
  readonly is_moderator: boolean | null;
  readonly created_at: string;
  readonly updated_at: string;
}

export interface PortfolioItemRow {
  readonly id: string;
  readonly developer_id: string;
  title: string;
  description: string | null;
  project_url: string | null;
  readonly created_at: string;
  readonly updated_at: string;
}

export interface CompanyMemberRow {
  readonly company_id: string;
  readonly user_id: string;
  readonly member_role: string;
  readonly created_at: string;
}

/** Only user-editable fields may be included in a profile mutation. */
export type ProfileUpdate = Pick<
  ProfileRow,
  'name' | 'bio' | 'skills' | 'github_url' | 'avatar_url'
>;
export type PortfolioItemUpdate = Pick<PortfolioItemRow, 'title' | 'description' | 'project_url'>;

export interface CategoryRow {
  readonly id: string;
  name: string;
}
export interface CompanyRow {
  readonly id: string;
  name: string;
  category_id: string | null;
  city: string;
  address: string | null;
  phone: string | null;
  email: string | null;
  website_url: string | null;
  readonly website_checked_at: string | null;
  source_url: string | null;
  readonly website_status: string;
  readonly visibility: string;
  readonly created_at: string;
  readonly updated_at: string;
}
export interface FavoriteRow {
  readonly developer_id: string;
  readonly company_id: string;
  readonly created_at: string;
}
export interface CompanyContactRow {
  readonly developer_id: string;
  readonly company_id: string;
  notes: string | null;
  contacted_at: string | null;
  status: string;
  readonly updated_at: string;
}
export interface CorrectionRequestRow {
  readonly id: string;
  readonly company_id: string;
  readonly requested_by: string;
  message: string;
  readonly resolved_by: string | null;
  readonly resolution_note: string | null;
  readonly resolved_at: string | null;
  readonly status: string;
  readonly created_at: string;
}
export interface ModerationActionRow {
  readonly id: string;
  readonly moderator_id: string;
  readonly company_id: string | null;
  readonly order_id: string | null;
  readonly action: string;
  readonly reason: string | null;
  readonly created_at: string;
}
export interface ApplicationRow {
  readonly id: string;
  readonly order_id: string;
  readonly developer_id: string;
  message: string;
  proposed_price: number | null;
  proposed_deadline: string | null;
  readonly status: string;
  readonly created_at: string;
  readonly updated_at: string;
}
export interface OrderRow {
  readonly id: string;
  readonly created_by: string;
  readonly selected_application_id: string | null;
  readonly company_id: string;
  title: string;
  description: string;
  readonly status: string;
  readonly visibility: boolean;
  budget_min: number | null;
  budget_max: number | null;
  deadline: string | null;
  readonly created_at: string;
  readonly updated_at: string;
}
export type CompanyUpdate = Pick<
  CompanyRow,
  'name' | 'city' | 'category_id' | 'address' | 'phone' | 'email' | 'website_url' | 'source_url'
>;
export type ContactUpdate = Pick<CompanyContactRow, 'notes' | 'contacted_at' | 'status'>;
export type OrderUpdate = Pick<
  OrderRow,
  'title' | 'description' | 'budget_min' | 'budget_max' | 'deadline'
>;
export type ApplicationUpdate = Pick<
  ApplicationRow,
  'message' | 'proposed_price' | 'proposed_deadline'
>;
export type CorrectionCreate = Pick<CorrectionRequestRow, 'company_id' | 'message'>;
