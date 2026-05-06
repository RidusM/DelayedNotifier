CREATE TABLE IF NOT EXISTS users (
    id           UUID        PRIMARY KEY,
    name         TEXT        NOT NULL,          
    email        TEXT        UNIQUE,           
    telegram_id  BIGINT      UNIQUE,            
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_email ON users (email) WHERE email IS NOT NULL;
CREATE INDEX idx_users_telegram_id ON users (telegram_id) WHERE telegram_id IS NOT NULL;