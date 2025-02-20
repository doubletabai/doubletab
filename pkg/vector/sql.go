package vector

const (
	knowledgeSchemaSQL = `
CREATE TABLE IF NOT EXISTS knowledge (
	id SERIAL PRIMARY KEY,
	text TEXT,
	embedding VECTOR(1536)
)
`
	storeKnowledgeSQL = `
INSERT INTO knowledge
	(text, embedding)
VALUES
    ($1, $2)
`
	queryKnowledgeSQL = `
SELECT
	text
FROM knowledge
ORDER BY
	embedding <-> $1
LIMIT 1
`
	truncateKnowledgeSQL = `
DELETE FROM knowledge
`
)
