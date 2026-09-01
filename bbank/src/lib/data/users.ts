import 'server-only'
import { apiListOrEmpty, type Page } from '../apiClient'

export interface AdminUser {
    id: number
    email: string
    role: string
    status: string
    center_id?: number | null
    hospital_id?: number | null
    last_login_at?: string | null
    created_at: string
    invite_pending: boolean
}

/** The six roles, in the order the console offers them. */
export const ROLES = ['donor', 'staff', 'lab_tech', 'inventory_manager', 'hospital_user', 'admin'] as const

export async function listUsers(params?: { role?: string; status?: string; q?: string }): Promise<{ items: AdminUser[]; page?: Page }> {
    const qs = new URLSearchParams()
    if (params?.role) qs.set('role', params.role)
    if (params?.status) qs.set('status', params.status)
    if (params?.q) qs.set('q', params.q)
    qs.set('limit', '100')
    return apiListOrEmpty<AdminUser>(`/api/v1/users?${qs.toString()}`)
}
