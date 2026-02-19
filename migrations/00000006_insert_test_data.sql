INSERT INTO user_telegram_links (
    user_id,
    telegram_chat_id,
    telegram_username,
    linked_at
) VALUES (
    '2bb36677-5e97-49a9-94b0-4da3c1a26c1c'::uuid,
    123456789,
    '@test_user_telegram',
    NOW()
) ON CONFLICT (user_id) DO UPDATE
    SET telegram_chat_id = EXCLUDED.telegram_chat_id;

INSERT INTO user_email_links (
    user_id,
    email,
    verified,
    linked_at
) VALUES (
    '0da72fa3-7973-4ed9-ac1c-00bbf4d08c33'::uuid,
    'test.user@example.com',
    true,
    NOW()
) ON CONFLICT (user_id) DO UPDATE
    SET email = EXCLUDED.email;

INSERT INTO user_email_links (
    user_id,
    email,
    verified,
    linked_at
) VALUES (
    '6b00378a-8876-4db7-9896-c03ed21cfb7d'::uuid,
    'queue.test@example.com',
    true,
    NOW()
) ON CONFLICT (user_id) DO UPDATE
    SET email = EXCLUDED.email;
