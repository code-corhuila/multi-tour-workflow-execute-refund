REVOKE workflow_execute_refund_writer FROM workflow_execute_refund_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA workflow_execute_refund REVOKE SELECT, INSERT, UPDATE ON TABLES FROM workflow_execute_refund_writer;
ALTER DEFAULT PRIVILEGES IN SCHEMA workflow_execute_refund REVOKE SELECT ON TABLES FROM workflow_execute_refund_reader;
REVOKE ALL ON ALL TABLES IN SCHEMA workflow_execute_refund FROM workflow_execute_refund_reader, workflow_execute_refund_writer;
REVOKE ALL ON SCHEMA workflow_execute_refund FROM workflow_execute_refund_reader, workflow_execute_refund_writer;
