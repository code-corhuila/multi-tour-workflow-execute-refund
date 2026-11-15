-- Back to one key across tenants. It fails while two tenants hold the same key, on purpose: the global constraint cannot
-- come back over that data, and a person decides which saga keeps the key.
ALTER TABLE workflow_execute_refund.execute_refund_saga DROP CONSTRAINT IF EXISTS uq_execute_refund_saga_tenant_idempotency_key;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conname = 'uq_execute_refund_saga_idempotency_key'
                     AND conrelid = 'workflow_execute_refund.execute_refund_saga'::regclass) THEN
        ALTER TABLE workflow_execute_refund.execute_refund_saga
            ADD CONSTRAINT uq_execute_refund_saga_idempotency_key UNIQUE (idempotency_key);
    END IF;
END $$;
