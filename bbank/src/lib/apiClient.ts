import 'server-only'
import { cookies } from 'next/headers'
import { API_BASE } from './api'
import { ACCESS_COOKIE } from './session'

/**
 * The one way this app talks to the Go API.
 *
 * It exists because three things had to happen on every single call and were
 * being done ad hoc, or not at all:
 *
 *  1. **Forward the session.** The browser's cookies reach Next.js, not the API.
 *     Every server-to-server call has to re-attach them or the API sees an
 *     anonymous request and returns 401 (which is exactly what happened between
 *     WI-20 and this change).
 *  2. **Unwrap the envelope.** The layered handlers return `{data, page}` and
 *     `{error}` (TRD §6.2); the not-yet-migrated legacy ones still return bare
 *     arrays. Callers should not have to know which is which.
 *  3. **Idempotency-Key on mutations** (§6.4), so a double-submitted form cannot
 *     create two rows once WI-77 turns the server side on.
 */

export class ApiError extends Error {
    constructor(
        readonly status: number,
        readonly code: string,
        message: string,
        readonly requestId?: string,
    ) {
        super(message)
        this.name = 'ApiError'
    }

    /** No valid session. The caller should send the user to the login page. */
    get isUnauthenticated() { return this.status === 401 }
    /** Signed in, but not allowed. Distinct from 404, which hides existence. */
    get isForbidden() { return this.status === 403 }
    get isNotFound() { return this.status === 404 }
}

export interface Page { total: number; limit: number; offset: number }

interface Options {
    method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
    body?: unknown
    /** Replay protection for mutations (TRD §6.4). Omit for a fresh key. */
    idempotencyKey?: string
    /** Opt out of cookie forwarding, for genuinely public endpoints. */
    anonymous?: boolean
}

interface Envelope<T> { data?: T; page?: Page; error?: { code?: string; message?: string; request_id?: string } }

async function request<T>(path: string, opts: Options = {}): Promise<{ data: T; page?: Page }> {
    const { method = 'GET', body, idempotencyKey, anonymous } = opts

    const headers: Record<string, string> = { Accept: 'application/json' }
    if (body !== undefined) headers['Content-Type'] = 'application/json'
    // Only when the CALLER supplies one.
    //
    // This used to fall back to `crypto.randomUUID()` per request, which cannot
    // deduplicate anything: a double-submitted form produced two different keys
    // and two rows, while filling the replay table with entries nothing would
    // ever match. The key has to identify the user's INTENT, so it must be
    // minted where the intent is — when the form is rendered — which is what
    // migration 000016 says and what WI-77 will wire through.
    if (idempotencyKey && method !== 'GET' && method !== 'DELETE') {
        headers['Idempotency-Key'] = idempotencyKey
    }
    if (!anonymous) {
        const token = (await cookies()).get(ACCESS_COOKIE)?.value
        if (token) headers['Cookie'] = `${ACCESS_COOKIE}=${token}`
    }

    let res: Response
    try {
        res = await fetch(`${API_BASE}${path}`, {
            method,
            headers,
            body: body === undefined ? undefined : JSON.stringify(body),
            cache: 'no-store',
        })
    } catch (cause) {
        // A transport failure is not a 500 from the API — saying so would send
        // whoever reads the log looking in the wrong process.
        throw new ApiError(0, 'network_error', `cannot reach the API: ${String(cause)}`)
    }

    const text = await res.text()
    let parsed: unknown = undefined
    if (text) {
        try {
            parsed = JSON.parse(text)
        } catch {
            // A non-JSON body means an error page or a proxy, not our API.
            if (!res.ok) throw new ApiError(res.status, 'unexpected_response', text.slice(0, 200))
        }
    }

    if (!res.ok) {
        const err = (parsed as Envelope<T> | undefined)?.error
        throw new ApiError(
            res.status,
            err?.code ?? 'unknown',
            // Legacy handlers answer with a bare string; envelope handlers with
            // a safe message. Never surface a raw driver error to a page.
            err?.message ?? (typeof parsed === 'string' ? parsed : res.statusText),
            err?.request_id,
        )
    }

    // Envelope if it has a `data` key; otherwise the value itself, which is how
    // the not-yet-migrated legacy endpoints answer.
    if (parsed && typeof parsed === 'object' && 'data' in parsed) {
        const env = parsed as Envelope<T>
        return { data: env.data as T, page: env.page }
    }
    return { data: parsed as T }
}

/** GET returning the payload only. */
export async function apiGet<T>(path: string, opts?: Omit<Options, 'method' | 'body'>): Promise<T> {
    return (await request<T>(path, { ...opts, method: 'GET' })).data
}

/** GET returning the payload and its pagination block. */
export async function apiList<T>(path: string, opts?: Omit<Options, 'method' | 'body'>): Promise<{ items: T[]; page?: Page }> {
    const { data, page } = await request<T[]>(path, { ...opts, method: 'GET' })
    return { items: Array.isArray(data) ? data : [], page }
}

export async function apiPost<T>(path: string, body?: unknown, opts?: Omit<Options, 'method' | 'body'>): Promise<T> {
    return (await request<T>(path, { ...opts, method: 'POST', body })).data
}

export async function apiPut<T>(path: string, body?: unknown, opts?: Omit<Options, 'method' | 'body'>): Promise<T> {
    return (await request<T>(path, { ...opts, method: 'PUT', body })).data
}

/**
 * PATCH is the update verb on `/api/v1` (TRD §6.5), not PUT: these endpoints
 * take the fields being changed, not a whole replacement resource.
 */
export async function apiPatch<T>(path: string, body?: unknown, opts?: Omit<Options, 'method' | 'body'>): Promise<T> {
    return (await request<T>(path, { ...opts, method: 'PATCH', body })).data
}

/**
 * For read paths that would rather render an empty state than an error page.
 * Only swallows the error after deciding it is one the page can survive; an
 * unauthenticated call still throws, because rendering "0 donors" to someone
 * whose session expired is a lie.
 */
export async function apiListOrEmpty<T>(path: string): Promise<{ items: T[]; page?: Page }> {
    try {
        return await apiList<T>(path)
    } catch (e) {
        // Only OUR errors are absorbed. Next.js signals `redirect()`, `notFound()`
        // and dynamic-rendering decisions by throwing, so a blanket catch here
        // would quietly eat framework control flow — during the build it turned
        // "this page must be dynamic" into a logged failure and an empty list.
        if (!(e instanceof ApiError)) throw e
        // An expired session must not render as "0 donors". That is a lie, and
        // the caller needs to send the user to the login page instead.
        if (e.isUnauthenticated) throw e
        console.error(`[api] ${path} failed: ${e.status} ${e.code} — ${e.message}`)
        return { items: [] }
    }
}
