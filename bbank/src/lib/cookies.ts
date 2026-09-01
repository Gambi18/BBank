/**
 * Whether auth cookies carry the `Secure` attribute.
 *
 * The API decides this from `COOKIE_SECURE`, and the frontend re-sets the same
 * cookies onto its own origin — so if the two disagree, one of them is wrong.
 * Reading the same variable here keeps them in step: an http deployment that
 * sets `COOKIE_SECURE=false` for the API would otherwise get `Secure` added back
 * by Next.js, and the browser would silently drop every session cookie.
 *
 * Defaults to secure. The unsafe value has to be asked for explicitly.
 */
export const COOKIE_SECURE = process.env.COOKIE_SECURE
    ? process.env.COOKIE_SECURE === 'true'
    : process.env.NODE_ENV === 'production'
