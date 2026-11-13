-- "New Chat" starts a fresh conversation: messages belong to a thread, and a user has one current thread.
-- Older threads stay stored; the app shows and feeds the model only the current one.
ALTER TABLE users ADD COLUMN chat_thread INT NOT NULL DEFAULT 1;
ALTER TABLE chat_messages ADD COLUMN thread INT NOT NULL DEFAULT 1;
CREATE INDEX chat_messages_thread_idx ON chat_messages (user_id, thread, id DESC);
