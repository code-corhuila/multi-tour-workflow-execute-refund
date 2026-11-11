CREATE INDEX IF NOT EXISTS idx_execute_refund_saga_status_updated_at ON workflow_execute_refund.execute_refund_saga (saga_status, updated_at);
