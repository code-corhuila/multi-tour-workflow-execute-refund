-- What a restart needs to carry a saga on (HU-WFL-003): who asked, the refund to execute and the cash movement of step 2,
-- which step 3 and the reversal need. All nullable. Only ids, an amount and a reason: no name, e-mail or document of the customer.
-- Guarded, so applying the changeset again does nothing.
ALTER TABLE workflow_execute_refund.execute_refund_saga
    ADD COLUMN IF NOT EXISTS on_behalf_of            text,
    ADD COLUMN IF NOT EXISTS refund_authorization_id uuid,
    ADD COLUMN IF NOT EXISTS reservation_id          uuid,
    ADD COLUMN IF NOT EXISTS outflow_medium          text,
    ADD COLUMN IF NOT EXISTS amount_cents            bigint,
    ADD COLUMN IF NOT EXISTS business_date           date,
    ADD COLUMN IF NOT EXISTS reason                  text,
    ADD COLUMN IF NOT EXISTS cash_movement_id        uuid;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_execute_refund_saga_outflow_medium') THEN
        ALTER TABLE workflow_execute_refund.execute_refund_saga
            ADD CONSTRAINT chk_execute_refund_saga_outflow_medium CHECK (outflow_medium IN ('CASH', 'TRANSFER'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_execute_refund_saga_amount_cents') THEN
        ALTER TABLE workflow_execute_refund.execute_refund_saga
            ADD CONSTRAINT chk_execute_refund_saga_amount_cents CHECK (amount_cents > 0);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_execute_refund_saga_on_behalf_of') THEN
        ALTER TABLE workflow_execute_refund.execute_refund_saga
            ADD CONSTRAINT chk_execute_refund_saga_on_behalf_of CHECK (char_length(on_behalf_of) BETWEEN 1 AND 128);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_execute_refund_saga_reason') THEN
        ALTER TABLE workflow_execute_refund.execute_refund_saga
            ADD CONSTRAINT chk_execute_refund_saga_reason CHECK (char_length(reason) <= 500);
    END IF;
END
$$;
