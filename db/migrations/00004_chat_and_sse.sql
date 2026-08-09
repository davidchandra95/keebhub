-- +goose Up
CREATE TABLE conversations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    listing_id BIGINT NOT NULL,
    seller_id BIGINT NOT NULL,
    buyer_id BIGINT NOT NULL,
    seller_last_read_message_id BIGINT,
    buyer_last_read_message_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_message_at TIMESTAMPTZ,
    CONSTRAINT conversations_listing_seller_fkey
        FOREIGN KEY (listing_id, seller_id) REFERENCES listings (id, seller_id),
    CONSTRAINT conversations_seller_fkey FOREIGN KEY (seller_id) REFERENCES users (id),
    CONSTRAINT conversations_buyer_fkey FOREIGN KEY (buyer_id) REFERENCES users (id),
    CONSTRAINT conversations_distinct_participants CHECK (seller_id <> buyer_id),
    CONSTRAINT conversations_unique_participants UNIQUE (listing_id, seller_id, buyer_id),
    CONSTRAINT conversations_last_message_after_created CHECK (last_message_at IS NULL OR last_message_at >= created_at)
);

CREATE INDEX conversations_seller_inbox_idx
    ON conversations (seller_id, (COALESCE(last_message_at, created_at)) DESC, id DESC);
CREATE INDEX conversations_buyer_inbox_idx
    ON conversations (buyer_id, (COALESCE(last_message_at, created_at)) DESC, id DESC);
CREATE INDEX conversations_listing_id_idx ON conversations (listing_id);

CREATE TABLE messages (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations (id),
    sender_id BIGINT NOT NULL REFERENCES users (id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT messages_body_length CHECK (char_length(body) BETWEEN 1 AND 2000),
    CONSTRAINT messages_id_conversation_id_key UNIQUE (id, conversation_id)
);

CREATE INDEX messages_conversation_id_id_idx ON messages (conversation_id, id);

ALTER TABLE conversations
    ADD CONSTRAINT conversations_seller_read_message_fkey
        FOREIGN KEY (seller_last_read_message_id, id) REFERENCES messages (id, conversation_id),
    ADD CONSTRAINT conversations_buyer_read_message_fkey
        FOREIGN KEY (buyer_last_read_message_id, id) REFERENCES messages (id, conversation_id);

-- +goose Down
ALTER TABLE conversations
    DROP CONSTRAINT conversations_seller_read_message_fkey,
    DROP CONSTRAINT conversations_buyer_read_message_fkey;
DROP TABLE messages;
DROP TABLE conversations;
