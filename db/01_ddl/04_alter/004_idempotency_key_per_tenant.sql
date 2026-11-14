-- The Idempotency-Key is unique per tenant, no longer across tenants (HU-WFL-003 AC-WF3-06 amended, DD-23): a key used by
-- one tenant must neither refuse nor reveal anything to another. NULLS NOT DISTINCT keeps a row without tenant unique
-- too. Guarded: applying it again does nothing.
ALTER TABLE workflow_execute_refund.execute_refund_saga DROP CONSTRAINT IF EXISTS uq_execute_refund_saga_idempotency_key;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conname = 'uq_execute_refund_saga_tenant_idempotency_key'
                     AND conrelid = 'workflow_execute_refund.execute_refund_saga'::regclass) THEN
        ALTER TABLE workflow_execute_refund.execute_refund_saga
            ADD CONSTRAINT uq_execute_refund_saga_tenant_idempotency_key UNIQUE NULLS NOT DISTINCT (tenant_id, idempotency_key);
    END IF;
END $$;
