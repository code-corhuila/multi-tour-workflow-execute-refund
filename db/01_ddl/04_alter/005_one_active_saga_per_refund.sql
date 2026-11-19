-- One refund authorization is paid at most once. Two sagas with different idempotency keys for the same authorization of a
-- tenant could both write a cash outflow; this index lets only one saga be RUNNING or COMPLETED per authorization.
-- A COMPENSATED or FAILED saga does not hold it, so a refund that did not go out can be tried again.
-- Guarded, so applying the changeset again does nothing. It fails while the data already holds two such sagas, on purpose:
-- a person decides which one stays.
CREATE UNIQUE INDEX IF NOT EXISTS uq_execute_refund_saga_active_refund
    ON workflow_execute_refund.execute_refund_saga (tenant_id, refund_authorization_id)
    WHERE saga_status IN ('RUNNING', 'COMPLETED') AND refund_authorization_id IS NOT NULL;
