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

const EMBEDDING_MODEL  = "gemini-embedding-2"; // Gemini embedding model
const VECTOR_SIZE      = 3072;                 // gemini-embedding-2 output dim
const BATCH_SIZE       = 50;                   // batch size for upsert and embed
const BATCH_DELAY_MS   = 5000;                 // delay between batches
const RETRY_DELAY_MS   = 30_000;               // retry delay on error

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

// gemini-embedding-2 accepts up to 2048 tokens ≈ ~8000 chars; truncate to be safe
const MAX_EMBED_CHARS = 7500;

async function embedBatch(texts: string[]): Promise<number[][]> {
  const model = genAI.getGenerativeModel({ model: EMBEDDING_MODEL });

  const attempt = async (): Promise<number[][]> => {
    const reqs = texts.map((t) => ({
      model: `models/${EMBEDDING_MODEL}`,
      content: {
        parts: [{ text: t.length > MAX_EMBED_CHARS ? t.slice(0, MAX_EMBED_CHARS) : t }],
      },
    }));

    const res = await model.batchEmbedContents({ requests: reqs });
    return res.embeddings.map((e) => e.values);
  };

  let lastErr: unknown;
  for (let attemptNum = 0; attemptNum < 3; attemptNum++) {
    try {
      return await attempt();
    } catch (err) {
      lastErr = err;
      const msg = err instanceof Error ? err.message : String(err);
      const isDailyQuota = msg.toLowerCase().includes("quota") || msg.toLowerCase().includes("exceeded your current quota");
      if (isDailyQuota) {
        console.error(`\n[FATAL] Gemini API Daily Quota Exceeded. Please wait for reset (PST midnight / 1:30 PM IST) or use a different API key.`);
        process.exit(1);
      }
      const wait = msg.includes("429") ? RETRY_DELAY_MS : 5_000;
      console.warn(`  ⚠  Batch embed error (attempt ${attemptNum + 1}): ${msg.slice(0, 120)}. Waiting ${wait / 1000}s…`);
      await sleep(wait);
    }
  }

  throw lastErr;
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

async function getExistingChunkIds(): Promise<Set<string>> {
  console.log(`Checking existing points in Qdrant collection "${COLLECTION_NAME}"...`);
  const existingIds = new Set<string>();
  let offset: string | number | null | undefined = undefined;

  while (true) {
    const res = await qdrant.scroll(COLLECTION_NAME, {
      limit: 1000,
      with_payload: ["chunk_id"],
      with_vector: false,
      offset: offset ?? undefined,
    });

    for (const point of res.points) {
      const payload = point.payload as any;
      if (payload && typeof payload.chunk_id === "string") {
        existingIds.add(payload.chunk_id);
      }
    }

    if (!res.next_page_offset) {
      break;
    }
    offset = res.next_page_offset;
  }

  return existingIds;
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
    console.log(`\nTotal chunks loaded: ${allChunks.length}`);

    // 3. Get already uploaded chunks
    const existingChunkIds = await getExistingChunkIds();
    console.log(`Found ${existingChunkIds.size} chunks already uploaded.`);

    const chunksToUpload = allChunks.filter((c) => !existingChunkIds.has(c.chunk_id));
    console.log(`Remaining chunks to embed & upload: ${chunksToUpload.length}`);

    if (chunksToUpload.length === 0) {
      console.log(`\n✓ All chunks are already uploaded! Nothing to do.`);
      console.log(`\n${"─".repeat(50)}\n`);
      return;
    }

    // 4. Embed + upload in batches
    console.log(`\nUploading in batches of ${BATCH_SIZE}…\n`);
    await uploadChunks(chunksToUpload);

    console.log(`\n${"─".repeat(50)}`);
    console.log(`✓ Done. ${chunksToUpload.length} points upserted to "${COLLECTION_NAME}".`);
    console.log(`${"─".repeat(50)}\n`);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.error(`\n[FATAL] ${msg}`);
    process.exit(1);
  }
}

main();
