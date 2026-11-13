DROP INDEX IF EXISTS chat_messages_thread_idx;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS thread;
ALTER TABLE users DROP COLUMN IF EXISTS chat_thread;
