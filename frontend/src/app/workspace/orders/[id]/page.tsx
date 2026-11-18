import { WorkspacePage } from '@/features/workspace/workspace-page';

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <WorkspacePage id={id} view="order" />;
}
