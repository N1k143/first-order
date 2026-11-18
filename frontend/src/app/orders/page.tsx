import {Suspense} from 'react';
import {OrdersPage} from '@/features/orders/orders-page';
export default function Page(){return <Suspense fallback={<div className="container page">Загрузка…</div>}><OrdersPage /></Suspense>}
