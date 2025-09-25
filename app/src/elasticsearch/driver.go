package elasticsearch

// @FIXME When a message is deleted, your application performs an update in Elasticsearch to add a deleted_at timestamp to the document.
//   All search queries from your application must be modified to filter out these documents (e.g., must_not: { exists: { field: "deleted_at" } }).
// @FIXME When storing messages, use ?_source_excludes=content parameter in the URL to avoid leaking cleartext content in _source field.
//   Also, use message_id "https://es-coordinating-1:9200/chat-messages/_doc/${message_id}"
