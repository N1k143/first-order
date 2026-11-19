import type {Application,Order} from '@/types/domain';
export const orderStatus:Record<Order['status'],string>={draft:'Черновик',open:'Приём откликов',in_progress:'Исполнитель выбран',completed:'Завершён',closed:'Закрыт',hidden:'Скрыт'};
export const applicationStatus:Record<Application['status'],string>={pending:'Отклик отправлен',selected:'Исполнитель выбран',rejected:'Отклонён',withdrawn:'Отозван',completed:'Завершён'};
