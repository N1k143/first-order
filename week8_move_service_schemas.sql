-- FirstOrder: align database objects with the five service-owned schemas.
-- Run after week8_data_model_fix.sql.

begin;

-- Profiles service
alter table public.profiles set schema profiles;
alter table public.portfolio_items set schema profiles;
alter table public.company_members set schema profiles;

-- Catalog service
alter table public.favorites set schema catalog;
alter table public.company_contacts set schema catalog;
alter table public.moderation_actions set schema catalog;

-- Recreate the business operation because PL/pgSQL table names in the
-- function body are not automatically rewritten by ALTER TABLE SET SCHEMA.
create or replace function orders.select_application(
  p_order_id uuid,
  p_application_id uuid
)
returns void
language plpgsql
security definer
set search_path = profiles, catalog, orders, applications, public, pg_temp
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
    from profiles.company_members
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

commit;
