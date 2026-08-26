CREATE TABLE message_questions (
    message_id INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
    questions_json TEXT NOT NULL CHECK (json_valid(questions_json))
);
