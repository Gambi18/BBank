// Single source of truth for the Go API base URL.
// In Docker, set API_BASE_URL=http://goapp:8000 (services can't reach each other via localhost).
// Locally it falls back to the host-mapped port.
export const API_BASE = process.env.API_BASE_URL || 'http://localhost:8000'

export const api = (path: string) => `${API_BASE}${path}`
