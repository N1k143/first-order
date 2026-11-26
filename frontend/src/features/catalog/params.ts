export function catalogParams(value: string) {
  const params = new URLSearchParams(value);
  if (!params.has('status') && params.has('website_status'))
    params.set('status', params.get('website_status') || '');
  params.delete('website_status');
  return params;
}
