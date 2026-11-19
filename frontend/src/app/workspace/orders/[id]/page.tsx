import {ManageOrderPage} from '@/features/workspace/orders-workspace';
export default async function Page({params}:{params:Promise<{id:string}>}){const {id}=await params;return <ManageOrderPage id={id}/>;}
