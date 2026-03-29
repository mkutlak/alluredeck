import { apiClient } from './client'
import type {
  Webhook,
  WebhookDelivery,
  CreateWebhookRequest,
  UpdateWebhookRequest,
} from '@/types/api'

export async function fetchWebhooks(projectId: string): Promise<Webhook[]> {
  const { data } = await apiClient.get<{ data: Webhook[] }>(
    `/projects/${projectId}/webhooks`,
  )
  return data.data
}

export async function createWebhook(
  projectId: string,
  req: CreateWebhookRequest,
): Promise<Webhook> {
  const { data } = await apiClient.post<{ data: Webhook }>(
    `/projects/${projectId}/webhooks`,
    req,
  )
  return data.data
}

export async function updateWebhook(
  projectId: string,
  webhookId: string,
  req: UpdateWebhookRequest,
): Promise<void> {
  await apiClient.put(`/projects/${projectId}/webhooks/${webhookId}`, req)
}

export async function deleteWebhook(
  projectId: string,
  webhookId: string,
): Promise<void> {
  await apiClient.delete(`/projects/${projectId}/webhooks/${webhookId}`)
}

export async function testWebhook(
  projectId: string,
  webhookId: string,
): Promise<string> {
  const { data } = await apiClient.post<{ data: { message: string } }>(
    `/projects/${projectId}/webhooks/${webhookId}/test`,
  )
  return data.data.message
}

export async function fetchWebhookDeliveries(
  projectId: string,
  webhookId: string,
  page = 1,
  perPage = 20,
): Promise<{ data: WebhookDelivery[]; total: number }> {
  const { data } = await apiClient.get<{
    data: WebhookDelivery[]
    metadata: { total: number }
  }>(`/projects/${projectId}/webhooks/${webhookId}/deliveries`, {
    params: { page, per_page: perPage },
  })
  return { data: data.data, total: data.metadata.total }
}
