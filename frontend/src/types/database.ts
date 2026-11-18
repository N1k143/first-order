/** Schema supplied by the project owner. These are row shapes, not API endpoints. */
export interface ProfileRow {
  readonly user_id: string;
  readonly role: string;
  name: string;
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
export type ProfileUpdate = Pick<ProfileRow, 'name' | 'bio' | 'skills' | 'github_url'>;
export type PortfolioItemUpdate = Pick<PortfolioItemRow, 'title' | 'description' | 'project_url'>;
