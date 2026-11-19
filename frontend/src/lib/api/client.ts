export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly details?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}
export interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
  timeoutMs?: number;
}
let refreshInFlight: Promise<void> | undefined;

/** Refresh cookies once for concurrent expired-session requests. Tokens stay HttpOnly. */
export function refreshSession(): Promise<void> {
  if (!refreshInFlight) {
    refreshInFlight = request<void>('/auth/refresh', { method: 'POST' }, false).finally(() => {
      refreshInFlight = undefined;
    });
  }
  return refreshInFlight;
}

/** Relative, same-origin requests. Session cookies never enter localStorage. */
export function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  return request<T>(path, options, true);
}
async function request<T>(path: string, options: RequestOptions, mayRefresh: boolean): Promise<T> {
  if (!path.startsWith('/') || path.startsWith('//') || path.includes('://')) {
    throw new Error('API path must be relative to the configured backend');
  }
  const { body, timeoutMs = 15000, signal, ...init } = options;
  const headers = new Headers(init.headers);
  if (body !== undefined) headers.set('Content-Type', 'application/json');
  headers.set('Accept', 'application/json, text/plain');
  const timeout = AbortSignal.timeout(timeoutMs);
  const response = await fetch(`/api${path}`, {
    ...init,
    headers,
    credentials: 'include',
    cache: 'no-store',
    signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const text = response.status === 204 ? '' : await response.text();
  let payload: unknown = text || undefined;
  if (text && response.headers.get('content-type')?.includes('application/json')) {
    try {
      payload = JSON.parse(text);
    } catch {
      throw new ApiError(response.status, 'Сервер вернул некорректный ответ. Попробуйте позже.');
    }
  }
  if (response.status === 401 && mayRefresh && !path.startsWith('/auth/') && !signal?.aborted) {
    try {
      await refreshSession();
    } catch {
      throw new ApiError(401, 'Войдите в аккаунт, чтобы продолжить.', payload);
    }
    // A 401 occurs before a protected handler runs. Other mutation failures are never retried.
    return request<T>(path, options, false);
  }
  if (!response.ok) {
    const messages: Record<number, string> = {
      400: 'Проверьте введённые данные.',
      401: 'Войдите в аккаунт, чтобы продолжить.',
      403: 'Недостаточно прав для этого действия.',
      404: 'Запрошенный ресурс не найден.',
      409: 'Данные уже изменились. Обновите страницу.',
      422: 'Проверьте заполнение формы.',
      429: 'Слишком много запросов. Попробуйте немного позже.',
    };
    const envelope = payload as { error?: { message?: unknown } } | undefined;
    const message =
      typeof envelope?.error?.message === 'string'
        ? envelope.error.message
        : messages[response.status];
    throw new ApiError(
      response.status,
      message || 'Не удалось получить данные. Попробуйте позже.',
      payload,
    );
  }
  return payload as T;
}
export function checkBackendHealth(signal?: AbortSignal) {
  return apiRequest<string>('/health', { signal });
}
