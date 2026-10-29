-- name: RecordFeishuInboundReceipt :exec
-- Rebind replaces config with a new generation and no receipt. Legacy configs
-- without a generation match only other legacy snapshots, never a new binding.
UPDATE channel_installation
SET config = jsonb_set(config, '{first_message_received_at}', to_jsonb(now()))
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
  AND channel_type = 'feishu'
  AND status = 'active'
  AND config->>'app_id' = sqlc.arg('app_id')::text
  AND COALESCE(config->>'binding_generation', '') = sqlc.arg('binding_generation')::text
  AND config->>'first_message_received_at' IS NULL;

-- name: SetFeishuInstallationBotUnionID :exec
-- Preserve concurrently recorded receipt evidence and binding generation.
UPDATE channel_installation
SET config = jsonb_set(config, '{bot_union_id}', to_jsonb(sqlc.arg('bot_union_id')::text)),
    updated_at = now()
WHERE id = sqlc.arg('id') AND channel_type = 'feishu';
