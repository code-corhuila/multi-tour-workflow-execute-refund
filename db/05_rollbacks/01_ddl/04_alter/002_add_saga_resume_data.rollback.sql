ALTER TABLE workflow_execute_refund.execute_refund_saga
    DROP CONSTRAINT IF EXISTS chk_execute_refund_saga_on_behalf_of,
    DROP CONSTRAINT IF EXISTS chk_execute_refund_saga_reason,
    DROP CONSTRAINT IF EXISTS chk_execute_refund_saga_amount_cents,
    DROP CONSTRAINT IF EXISTS chk_execute_refund_saga_outflow_medium,
    DROP COLUMN IF EXISTS cash_movement_id,
    DROP COLUMN IF EXISTS reason,
    DROP COLUMN IF EXISTS business_date,
    DROP COLUMN IF EXISTS amount_cents,
    DROP COLUMN IF EXISTS outflow_medium,
    DROP COLUMN IF EXISTS reservation_id,
    DROP COLUMN IF EXISTS refund_authorization_id,
    DROP COLUMN IF EXISTS on_behalf_of;
