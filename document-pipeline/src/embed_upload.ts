import "dotenv/config";
import fs from "fs";
import path from "path";
import { GoogleGenerativeAI } from "@google/generative-ai";
import { QdrantClient } from "@qdrant/js-client-rest";
import { Chunk } from "./types";

// ─── Config ───────────────────────────────────────────────────────────────────

const GEMINI_API_KEY   = process.env.GEMINI_API_KEY   ?? "";
const QDRANT_URL       = process.env.QDRANT_URL        ?? "http://localhost:6333";
const COLLECTION_NAME  = process.env.COLLECTION_NAME   ?? "ipsakti_docs";

const EMBEDDING_MODEL  = "text-embedding-004"; // Gemini embedding model
const VECTOR_SIZE      = 768;                  // text-embedding-004 output dim
const BATCH_SIZE       = 50;
const BATCH_DELAY_MS   = 200;
const RETRY_DELAY_MS   = 10_000;

const CHUNKS_DIR = path.resolve(__dirname, "../chunks");

if (!GEMINI_API_KEY) {
  console.error("[ERROR] GEMINI_API_KEY is not set in .env");
  process.exit(1);
}

// ─── Clients ──────────────────────────────────────────────────────────────────

const genAI  = new GoogleGenerativeAI(GEMINI_API_KEY);
const qdrant = new QdrantClient({ url: QDRANT_URL });

// ─── Stable Hash (djb2) ───────────────────────────────────────────────────────
// Converts a chunk_id string → stable positive integer safe for Qdrant point IDs.
// Uses modulo 2^53 to stay within JS safe-integer range.

function djb2Hash(str: string): number {
  let hash = 5381;
  for (let i = 0; i < str.length; i++) {
    hash = (hash * 33) ^ str.charCodeAt(i);
  }
  return Math.abs(hash % Number.MAX_SAFE_INTEGER);
}

// ─── Qdrant Collection Setup ──────────────────────────────────────────────────

async function ensureCollection(): Promise<void> {
  const existing = await qdrant.getCollections();
  const exists   = existing.collections.some((c) => c.name === COLLECTION_NAME);

  if (exists) {
    console.log(`Collection "${COLLECTION_NAME}" already exists — skipping creation.`);
    return;
  }

  await qdrant.createCollection(COLLECTION_NAME, {
    vectors: {
      size:     VECTOR_SIZE,
      distance: "Cosine",
    },
  });
  console.log(`✓ Created collection "${COLLECTION_NAME}" (size=${VECTOR_SIZE}, Cosine)`);
}

// ─── Embedding ────────────────────────────────────────────────────────────────

async function embedBatch(texts: string[]): Promise<number[][]> {
  const model = genAI.getGenerativeModel({ model: EMBEDDING_MODEL });

  const attempt = async (): Promise<number[][]> => {
    const results: number[][] = [];
    for (const text of texts) {
      const res = await model.embedContent(text);
      results.push(res.embedding.values);
    }
    return results;
  };

  try {
    return await attempt();
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.warn(`  ⚠  Embedding error: ${msg}. Retrying in ${RETRY_DELAY_MS / 1000}s…`);
    await sleep(RETRY_DELAY_MS);
    return await attempt(); // throws if it fails again — caller handles it
  }
}

// ─── Upload ───────────────────────────────────────────────────────────────────

async function uploadChunks(chunks: Chunk[]): Promise<void> {
  const total = chunks.length;
  let uploaded = 0;

  for (let i = 0; i < total; i += BATCH_SIZE) {
    const batch  = chunks.slice(i, i + BATCH_SIZE);
    const texts  = batch.map((c) => c.text);
    const vectors = await embedBatch(texts);

    const points = batch.map((chunk, idx) => ({
      id:      djb2Hash(chunk.chunk_id),
      vector:  vectors[idx],
      payload: {
        chunk_id:    chunk.chunk_id,
        text:        chunk.text,
        doc_title:   chunk.doc_title,
        domain:      chunk.domain,
        jurisdiction:chunk.jurisdiction,
        authority:   chunk.authority,
        section_ref: chunk.section_ref,
        source_url:  chunk.source_url,
        retrieved_at:chunk.retrieved_at,
        char_start:  chunk.char_start,
        char_end:    chunk.char_end,
      } satisfies Chunk,
    }));

    await qdrant.upsert(COLLECTION_NAME, { wait: true, points });

    uploaded += batch.length;
    console.log(`  ${uploaded}/${total} uploaded`);

    if (i + BATCH_SIZE < total) {
      await sleep(BATCH_DELAY_MS);
    }
  }
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function loadAllChunks(): Chunk[] {
  const chunkFiles = fs
    .readdirSync(CHUNKS_DIR)
    .filter((f) => f.endsWith("_chunks.json"));

  if (chunkFiles.length === 0) {
    console.error(
      `[ERROR] No *_chunks.json files found in ${CHUNKS_DIR}.\n` +
      `Run "npm run chunk" first.`
    );
    process.exit(1);
  }

  const allChunks: Chunk[] = [];
  for (const file of chunkFiles) {
    const raw = fs.readFileSync(path.join(CHUNKS_DIR, file), "utf-8");
    const chunks: Chunk[] = JSON.parse(raw);
    allChunks.push(...chunks);
    console.log(`  Loaded ${chunks.length} chunks from ${file}`);
  }
  return allChunks;
}

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  console.log(`\n${"─".repeat(50)}`);
  console.log(`IP-SAKTI  ·  Embed & Upload`);
  console.log(`Collection : ${COLLECTION_NAME}  |  Qdrant: ${QDRANT_URL}`);
  console.log(`Model      : ${EMBEDDING_MODEL} (dim=${VECTOR_SIZE})`);
  console.log(`${"─".repeat(50)}\n`);

  try {
    // 1. Ensure Qdrant collection exists
    await ensureCollection();

    // 2. Load all chunks from disk
    console.log("\nLoading chunks from chunks/…");
    const allChunks = loadAllChunks();
    console.log(`\nTotal chunks to embed & upload: ${allChunks.length}`);

    // 3. Embed + upload in batches
    console.log(`\nUploading in batches of ${BATCH_SIZE}…\n`);
    await uploadChunks(allChunks);

    console.log(`\n${"─".repeat(50)}`);
    console.log(`✓ Done. ${allChunks.length} points upserted to "${COLLECTION_NAME}".`);
    console.log(`${"─".repeat(50)}\n`);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.error(`\n[FATAL] ${msg}`);
    process.exit(1);
  }
}

main();
