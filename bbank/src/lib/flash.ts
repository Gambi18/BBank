/**
 * Builds the `?success=` / `?error=` URLs that `components/ToastAlert.tsx` reads.
 *
 * It exists because these were hand-written as `?error=Some+message`, which is
 * correct only as long as every message happens to contain nothing but letters
 * and spaces. An em dash or an ampersand in one silently breaks the URL, and a
 * `+` passed to `URLSearchParams.set` comes back as a literal plus sign.
 */
export function flash(path: string, params: { error?: string; success?: string }): string {
    const q = new URLSearchParams()
    if (params.error) q.set('error', params.error)
    if (params.success) q.set('success', params.success)
    const qs = q.toString()
    return qs ? `${path}?${qs}` : path
}
