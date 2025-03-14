package vector

const (
	knowledgeSchemaSQL = `
CREATE TABLE IF NOT EXISTS knowledge (
	id SERIAL PRIMARY KEY,
	content TEXT NOT NULL,
	embedding VECTOR(%d) NOT NULL
)
`
	storeKnowledgeSQL = `
INSERT INTO knowledge
	(content, embedding)
VALUES
    ($1, $2)
`
	queryKnowledgeSQL = `
SELECT
	content
FROM knowledge
ORDER BY
	embedding <-> $1
LIMIT 1
`
	truncateKnowledgeSQL = `
DELETE FROM knowledge
`
	memorySchemaSQL = `
CREATE TABLE IF NOT EXISTS memory (
	id SERIAL PRIMARY KEY,
	session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL,
	embedding VECTOR(%d) NOT NULL
)
`
	storeMemorySQL = `
INSERT INTO memory
	(session_id, role, content, created_at, embedding)
VALUES
	(:session_id, :role, :content, :created_at, :embedding)
`
	queryMemorySQL = `
SELECT
	role, content
FROM memory
WHERE
	session_id = $1
ORDER BY
	created_at DESC,
	embedding <-> $2
LIMIT 5
`
)
