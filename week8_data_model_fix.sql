-- FirstOrder, week 8: corrective database migration.
-- Review and back up the database before running this file.
-- The whole migration is transactional: any error rolls everything back.

begin;

-- ---------------------------------------------------------------------------
-- 1. Profiles and roles
-- ---------------------------------------------------------------------------

alter table public.profiles
  alter column user_id drop default,
  alter column created_at set default now(),
  alter column created_at set not null,
  alter column updated_at set default now(),
  alter column updated_at set not null;

alter table public.profiles
  add column if not exists role text not null default 'developer';

update public.profiles
set role = 'moderator'
where is_moderator is true;

alter table public.profiles
  drop constraint if exists profiles_role_check,
  add constraint profiles_role_check
    check (role in ('developer', 'moderator'));

alter table public.profiles
  drop constraint if exists profiles_user_id_fkey,
  add constraint profiles_user_id_fkey
    foreign key (user_id) references auth.users(id) on delete cascade;

-- Keep is_moderator temporarily for API compatibility. Remove it after the
-- application starts using profiles.role.

-- ---------------------------------------------------------------------------
-- 2. Companies and company representatives
-- ---------------------------------------------------------------------------

-- orders.company_id currently points to company_members instead of companies.
alter table orders.orders
  drop constraint if exists orders_company_id_fkey;

alter table public.company_members
  drop constraint if exists company_members_pkey,
  drop constraint if exists company_members_company_id_fkey,
  drop constraint if exists company_members_user_id_fkey;

alter table public.company_members
  alter column company_id drop default,
  alter column user_id drop default,
  alter column member_role type text
    using case
      when member_role is true then 'owner'
      when member_role is false then 'representative'
      else 'representative'
    end,
  alter column member_role set default 'representative',
  alter column member_role set not null,
  alter column created_at set default now(),
  alter column created_at set not null;

alter table public.company_members
  add constraint company_members_pkey primary key (company_id, user_id),
  add constraint company_members_company_id_fkey
    foreign key (company_id) references catalog.companies(id) on delete cascade,
  add constraint company_members_user_id_fkey
    foreign key (user_id) references public.profiles(user_id) on delete cascade,
  add constraint company_members_role_check
    check (member_role in ('owner', 'representative'));

create index if not exists company_members_user_id_idx
  on public.company_members(user_id);

-- ---------------------------------------------------------------------------
-- 3. Orders
-- ---------------------------------------------------------------------------

update orders.orders
set status = coalesce(status, 'draft'),
    visibility = coalesce(visibility, true),
    created_at = coalesce(created_at, now()),
    updated_at = coalesce(updated_at, now());

alter table orders.orders
  alter column company_id drop default,
  alter column company_id set not null,
  alter column created_by drop default,
  alter column title set not null,
  alter column description set not null,
  alter column status set default 'draft',
  alter column status set not null,
  alter column visibility set default true,
  alter column visibility set not null,
  alter column selected_application_id drop default,
  alter column created_at set default now(),
  alter column created_at set not null,
  alter column updated_at set default now(),
  alter column updated_at set not null;

alter table orders.orders
  drop constraint if exists orders_company_id_fkey,
  drop constraint if exists orders_created_by_fkey,
  drop constraint if exists orders_status_check,
  drop constraint if exists orders_budget_check,
  add constraint orders_company_id_fkey
    foreign key (company_id) references catalog.companies(id) on delete restrict,
  add constraint orders_created_by_fkey
    foreign key (created_by) references public.profiles(user_id) on delete restrict,
  add constraint orders_status_check
    check (status in
      ('draft', 'open', 'contractor_selected', 'completed', 'closed', 'hidden')),
  add constraint orders_budget_check
    check (
      (budget_min is null or budget_min >= 0)
      and (budget_max is null or budget_max >= 0)
      and (budget_min is null or budget_max is null or budget_max >= budget_min)
    );

create index if not exists orders_company_id_idx on orders.orders(company_id);
create index if not exists orders_created_by_idx on orders.orders(created_by);
create index if not exists orders_status_visible_idx
  on orders.orders(status, visibility);

-- ---------------------------------------------------------------------------
-- 4. Applications and selection of one contractor
-- ---------------------------------------------------------------------------

alter table applications.applications
  add column if not exists status text not null default 'submitted';

alter table applications.applications
  alter column order_id drop default,
  alter column developer_id drop default,
  alter column created_at set default now(),
  alter column created_at set not null,
  alter column updated_at set default now(),
  alter column updated_at set not null;

alter table applications.applications
  drop constraint if exists applications_order_id_fkey,
  drop constraint if exists applications_developer_id_fkey,
  drop constraint if exists applications_status_check,
  add constraint applications_order_id_fkey
    foreign key (order_id) references orders.orders(id) on delete cascade,
  add constraint applications_developer_id_fkey
    foreign key (developer_id) references public.profiles(user_id) on delete cascade,
  add constraint applications_status_check
    check (status in ('submitted', 'accepted', 'rejected', 'withdrawn'));

-- Supports a composite FK that proves the selected application belongs to
-- exactly the same order.
create unique index if not exists applications_id_order_id_key
  on applications.applications(id, order_id);

create index if not exists applications_order_id_idx
  on applications.applications(order_id);
create index if not exists applications_developer_id_idx
  on applications.applications(developer_id);

alter table orders.orders
  drop constraint if exists orders_selected_application_fkey,
  add constraint orders_selected_application_fkey
    foreign key (selected_application_id, id)
    references applications.applications(id, order_id)
    on delete restrict;

-- ---------------------------------------------------------------------------
-- 5. Import subsystem
-- ---------------------------------------------------------------------------

alter table if exists importer.import_runsid rename to import_runs;

alter table importer.import_runs
  alter column processed_count type integer,
  alter column processed_count set default 0,
  alter column started_at set default now(),
  alter column started_at set not null;

alter table importer.import_runs
  add column if not exists finished_at timestamptz,
  drop constraint if exists import_runs_status_check,
  drop constraint if exists import_runs_processed_count_check,
  add constraint import_runs_status_check
    check (status in ('pending', 'running', 'completed', 'failed')),
  add constraint import_runs_processed_count_check
    check (processed_count >= 0);

alter table importer.source_records
  drop constraint if exists source_records_last_run_id_fkey,
  drop constraint if exists source_records_company_id_fkey,
  alter column company_id drop default,
  alter column last_run_id drop default,
  alter column last_seen_at set default now(),
  add constraint source_records_last_run_id_fkey
    foreign key (last_run_id) references importer.import_runs(id) on delete restrict,
  add constraint source_records_company_id_fkey
    foreign key (company_id) references catalog.companies(id) on delete set null;

create unique index if not exists source_records_source_external_id_key
  on importer.source_records(source, external_id);
create index if not exists source_records_company_id_idx
  on importer.source_records(company_id);

-- ---------------------------------------------------------------------------
-- 6. Missing entities required by the use cases
-- ---------------------------------------------------------------------------

create table if not exists public.portfolio_items (
  id uuid primary key default gen_random_uuid(),
  developer_id uuid not null
    references public.profiles(user_id) on delete cascade,
  title text not null,
  description text,
  project_url text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists portfolio_items_developer_id_idx
  on public.portfolio_items(developer_id);

create table if not exists public.favorites (
  developer_id uuid not null
    references public.profiles(user_id) on delete cascade,
  company_id uuid not null
    references catalog.companies(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (developer_id, company_id)
);

create table if not exists public.company_contacts (
  developer_id uuid not null
    references public.profiles(user_id) on delete cascade,
  company_id uuid not null
    references catalog.companies(id) on delete cascade,
  status text not null default 'planned'
    check (status in ('planned', 'contacted', 'replied', 'declined')),
  notes text,
  contacted_at timestamptz,
  updated_at timestamptz not null default now(),
  primary key (developer_id, company_id)
);

create table if not exists catalog.correction_requests (
  id uuid primary key default gen_random_uuid(),
  company_id uuid not null
    references catalog.companies(id) on delete cascade,
  requested_by uuid not null
    references public.profiles(user_id) on delete cascade,
  message text not null,
  status text not null default 'pending'
    check (status in ('pending', 'approved', 'rejected')),
  resolved_by uuid references public.profiles(user_id) on delete set null,
  resolution_note text,
  created_at timestamptz not null default now(),
  resolved_at timestamptz
);

create index if not exists correction_requests_company_id_idx
  on catalog.correction_requests(company_id);

create table if not exists public.moderation_actions (
  id uuid primary key default gen_random_uuid(),
  moderator_id uuid not null
    references public.profiles(user_id) on delete restrict,
  company_id uuid references catalog.companies(id) on delete set null,
  order_id uuid references orders.orders(id) on delete set null,
  action text not null
    check (action in ('publish', 'hide', 'correct', 'approve', 'reject')),
  reason text,
  created_at timestamptz not null default now(),
  constraint moderation_actions_target_check
    check (company_id is not null or order_id is not null)
);

-- ---------------------------------------------------------------------------
-- 7. Automatically maintain updated_at
-- ---------------------------------------------------------------------------

create or replace function public.set_updated_at()
returns trigger
language plpgsql
as $$
begin
  new.updated_at = now();
  return new;
end;
$$;

-- A user may edit their profile but may not promote themselves to moderator.
create or replace function public.prevent_profile_role_escalation()
returns trigger
language plpgsql
as $$
begin
  if new.role is distinct from old.role
     and coalesce(current_setting('request.jwt.claim.role', true), '')
         <> 'service_role' then
    raise exception 'Profile role may only be changed by the backend';
  end if;
  return new;
end;
$$;

drop trigger if exists profiles_set_updated_at on public.profiles;
create trigger profiles_set_updated_at
before update on public.profiles
for each row execute function public.set_updated_at();

drop trigger if exists profiles_prevent_role_escalation on public.profiles;
create trigger profiles_prevent_role_escalation
before update of role on public.profiles
for each row execute function public.prevent_profile_role_escalation();

drop trigger if exists companies_set_updated_at on catalog.companies;
create trigger companies_set_updated_at
before update on catalog.companies
for each row execute function public.set_updated_at();

drop trigger if exists orders_set_updated_at on orders.orders;
create trigger orders_set_updated_at
before update on orders.orders
for each row execute function public.set_updated_at();

drop trigger if exists applications_set_updated_at
  on applications.applications;
create trigger applications_set_updated_at
before update on applications.applications
for each row execute function public.set_updated_at();

drop trigger if exists portfolio_items_set_updated_at on public.portfolio_items;
create trigger portfolio_items_set_updated_at
before update on public.portfolio_items
for each row execute function public.set_updated_at();

drop trigger if exists company_contacts_set_updated_at on public.company_contacts;
create trigger company_contacts_set_updated_at
before update on public.company_contacts
for each row execute function public.set_updated_at();

-- ---------------------------------------------------------------------------
-- 8. Atomic contractor selection
-- ---------------------------------------------------------------------------

create or replace function orders.select_application(
  p_order_id uuid,
  p_application_id uuid
)
returns void
language plpgsql
security definer
set search_path = public, orders, applications, pg_temp
as $$
declare
  v_company_id uuid;
  v_status text;
begin
  select company_id, status
    into v_company_id, v_status
  from orders.orders
  where id = p_order_id
  for update;

  if not found then
    raise exception 'Order not found';
  end if;

  if v_status <> 'open' then
    raise exception 'Only an open order can select a contractor';
  end if;

  if not exists (
    select 1
    from public.company_members
    where company_id = v_company_id
      and user_id = auth.uid()
  ) then
    raise exception 'Not allowed to manage this order';
  end if;

  if not exists (
    select 1
    from applications.applications
    where id = p_application_id
      and order_id = p_order_id
      and status = 'submitted'
  ) then
    raise exception 'Submitted application not found for this order';
  end if;

  update applications.applications
  set status = case
      when id = p_application_id then 'accepted'
      else 'rejected'
    end,
    updated_at = now()
  where order_id = p_order_id
    and status = 'submitted';

  update orders.orders
  set selected_application_id = p_application_id,
      status = 'contractor_selected',
      updated_at = now()
  where id = p_order_id;
end;
$$;

revoke all on function orders.select_application(uuid, uuid) from public;
grant execute on function orders.select_application(uuid, uuid) to authenticated;

-- ---------------------------------------------------------------------------
-- 9. Baseline RLS policies
-- ---------------------------------------------------------------------------

alter table public.portfolio_items enable row level security;
alter table public.favorites enable row level security;
alter table public.company_contacts enable row level security;
alter table catalog.correction_requests enable row level security;
alter table public.moderation_actions enable row level security;

drop policy if exists profiles_read on public.profiles;
create policy profiles_read on public.profiles
  for select to authenticated using (true);
drop policy if exists profiles_insert_own on public.profiles;
create policy profiles_insert_own on public.profiles
  for insert to authenticated
  with check (user_id = auth.uid() and role = 'developer');
drop policy if exists profiles_update_own on public.profiles;
create policy profiles_update_own on public.profiles
  for update to authenticated
  using (user_id = auth.uid())
  with check (user_id = auth.uid());

drop policy if exists company_members_read on public.company_members;
create policy company_members_read on public.company_members
  for select to authenticated using (true);

drop policy if exists portfolio_read on public.portfolio_items;
create policy portfolio_read on public.portfolio_items
  for select to authenticated using (true);
drop policy if exists portfolio_manage_own on public.portfolio_items;
create policy portfolio_manage_own on public.portfolio_items
  for all to authenticated
  using (developer_id = auth.uid())
  with check (developer_id = auth.uid());

drop policy if exists favorites_manage_own on public.favorites;
create policy favorites_manage_own on public.favorites
  for all to authenticated
  using (developer_id = auth.uid())
  with check (developer_id = auth.uid());

drop policy if exists contacts_manage_own on public.company_contacts;
create policy contacts_manage_own on public.company_contacts
  for all to authenticated
  using (developer_id = auth.uid())
  with check (developer_id = auth.uid());

-- The Go backend may continue using service_role for the private schemas.
-- service_role bypasses RLS. Do not expose that key in the browser.

commit;
