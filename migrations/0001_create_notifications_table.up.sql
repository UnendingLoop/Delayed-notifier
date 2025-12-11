-- migrations/0001_create_notifications_table.up.sql
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4 (),
    recipient TEXT, -- кто получет (можно user_id или email)
    channel TEXT, -- 'email' | 'telegram' | 'console' ...
    payload JSONB, -- произвольная полезная нагрузка
    text TEXT, -- дублируем для простоты поиска/лога
    send_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'planned', -- planned|queued|delivered|failed|cancelled
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notifications_send_at_status ON notifications (send_at, status);