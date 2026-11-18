import { OrderEditor } from '@/features/workspace/order-editor';

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <OrderEditor id={id} preview />;
}
