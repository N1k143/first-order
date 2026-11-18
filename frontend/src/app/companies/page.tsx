import {Suspense} from 'react';
import {CatalogPage} from '@/features/catalog/catalog-page';
export default function Page(){return <Suspense fallback={<div className="container page">Загрузка…</div>}><CatalogPage /></Suspense>}
