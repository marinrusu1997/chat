// This file defines the optimized schema for the recommendation engine.

// --- User Schema ---
// Ensures every user has a unique ID (and creates an index).
CREATE CONSTRAINT user_id_unique IF NOT EXISTS FOR (u:User) REQUIRE u.id IS UNIQUE;
// Ensures every user node has a name property.
CREATE CONSTRAINT user_name_exists IF NOT EXISTS FOR (u:User) REQUIRE u.name IS NOT NULL;


// --- Chat Schema ---
// Ensures every chat has a unique ID (and creates an index).
CREATE CONSTRAINT chat_id_unique IF NOT EXISTS FOR (c:Chat) REQUIRE c.id IS UNIQUE;

// Add property existence constraints for core chat properties.
CREATE CONSTRAINT chat_type_exists IF NOT EXISTS FOR (c:Chat) REQUIRE c.type IS NOT NULL;
CREATE CONSTRAINT chat_visibility_exists IF NOT EXISTS FOR (c:Chat) REQUIRE c.visibility IS NOT NULL;
CREATE CONSTRAINT chat_status_exists IF NOT EXISTS FOR (c:Chat) REQUIRE c.status IS NOT NULL;


// --- Optimized Index for Recommendations ---
// This composite index is specifically designed to make searches for
// active, public group chats extremely fast.
CREATE INDEX chat_recommendation_index IF NOT EXISTS FOR (c:Chat) ON (c.type, c.visibility, c.status);