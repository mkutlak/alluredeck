import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import {
  createWebhook,
  deleteWebhook,
  fetchWebhookDeliveries,
  fetchWebhooks,
  testWebhook,
  updateWebhook,
} from '@/api/webhooks'
import { toast } from '@/components/ui/use-toast'
import { queryKeys } from '@/lib/query-keys'
import type {
  CreateWebhookRequest,
  UpdateWebhookRequest,
  Webhook,
  WebhookDelivery,
} from '@/types/api'

export const DELIVERIES_PER_PAGE = 10

export function useWebhookListQuery(projectId: string) {
  return useQuery({
    queryKey: queryKeys.webhooks(projectId),
    queryFn: () => fetchWebhooks(projectId),
    enabled: Boolean(projectId),
  })
}

export function useCreateWebhook(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateWebhookRequest) => createWebhook(projectId, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.webhooks(projectId) })
      toast({ title: 'Webhook created' })
    },
    onError: () => {
      toast({ title: 'Failed to create webhook', variant: 'destructive' })
    },
  })
}

export function useUpdateWebhook(projectId: string, webhookId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: UpdateWebhookRequest) => updateWebhook(projectId, webhookId, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.webhooks(projectId) })
      toast({ title: 'Webhook updated' })
    },
    onError: () => {
      toast({ title: 'Failed to update webhook', variant: 'destructive' })
    },
  })
}

export function useDeleteWebhook(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteWebhook(projectId, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.webhooks(projectId) })
      toast({ title: 'Webhook deleted' })
    },
    onError: () => {
      toast({ title: 'Failed to delete webhook', variant: 'destructive' })
    },
  })
}

export function useTestWebhook(projectId: string, webhookId: string) {
  return useMutation({
    mutationFn: () => testWebhook(projectId, webhookId),
    onSuccess: () => {
      toast({ title: 'Test delivery sent' })
    },
    onError: () => {
      toast({ title: 'Test delivery failed', variant: 'destructive' })
    },
  })
}

export function useDeliveryHistory(
  projectId: string,
  webhook: Webhook | null,
  page: number,
) {
  return useQuery({
    queryKey: queryKeys.webhookDeliveries(projectId, String(webhook?.id ?? ''), page),
    queryFn: () =>
      fetchWebhookDeliveries(projectId, String(webhook!.id), page, DELIVERIES_PER_PAGE),
    enabled: webhook !== null,
  })
}

export type { Webhook, WebhookDelivery, CreateWebhookRequest, UpdateWebhookRequest }
