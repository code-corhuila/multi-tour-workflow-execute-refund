GRANT USAGE ON SCHEMA workflow_execute_refund TO workflow_execute_refund_reader, workflow_execute_refund_writer;
GRANT SELECT ON ALL TABLES IN SCHEMA workflow_execute_refund TO workflow_execute_refund_reader;
-- No DELETE: a saga is never removed by the service, the state is the audit of the process
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA workflow_execute_refund TO workflow_execute_refund_writer;
ALTER DEFAULT PRIVILEGES IN SCHEMA workflow_execute_refund GRANT SELECT ON TABLES TO workflow_execute_refund_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA workflow_execute_refund GRANT SELECT, INSERT, UPDATE ON TABLES TO workflow_execute_refund_writer;
GRANT workflow_execute_refund_writer TO workflow_execute_refund_app;
