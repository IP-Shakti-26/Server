import "dotenv/config";
import fs from "fs";
import path from "path";
import { QdrantClient } from "@qdrant/js-client-rest";
import { Chunk } from "./types";

// ─── Config ───────────────────────────────────────────────────────────────────

const OLLAMA_URL       = process.env.OLLAMA_URL        ?? "http://localhost:11434";
const QDRANT_URL       = process.env.QDRANT_URL        ?? "http://localhost:6333";
const COLLECTION_NAME  = process.env.COLLECTION_NAME   ?? "ipsakti_docs";

const EMBEDDING_MODEL  = "nomic-ipsakti"; // custom model: nomic-embed-text:v1.5 + num_ctx 4096
const VECTOR_SIZE      = 768;                    // nomic-embed-text output dim
const BATCH_SIZE       = 10;                     // batch size for upsert and embed
const BATCH_DELAY_MS   = 100;                    // local server delay
// nomic-embed-text:v1.5 default num_ctx=2048 tokens. After sanitizing mojibake, real legal text
// at ~8000 chars is ~1800-2200 tokens — within the limit. Keep a safe buffer below 2048.
const MAX_CHARS        = 7_500;

const CHUNKS_DIR = path.resolve(__dirname, "../chunks");

// ─── Clients ──────────────────────────────────────────────────────────────────

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
    // Check vector size
    const info = await qdrant.getCollection(COLLECTION_NAME);
    const currentSize = (info.config.params.vectors as any)?.size;
    if (currentSize === VECTOR_SIZE) {
      console.log(`Collection "${COLLECTION_NAME}" already exists with correct dimensions (${VECTOR_SIZE}) — skipping creation.`);
      return;
    }

    console.log(`Collection "${COLLECTION_NAME}" has incorrect dimension (${currentSize}). Deleting and recreating for dimension ${VECTOR_SIZE}...`);
    await qdrant.deleteCollection(COLLECTION_NAME);
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

/**
 * Strip non-printable control characters and mojibake artifacts from PDF text.
 * Replaces control chars (except tab, newline, carriage return) and U+FFFD with a space.
 * Also collapses excessive whitespace.
 */
function sanitizeText(text: string): string {
  return text
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F\uFFFD]/g, " ")
    .replace(/\s{3,}/g, "  ")
    .trim();
}

/**
 * Returns true if more than 30% of the text consists of non-ASCII / control characters —
 * a sign of corrupted PDF extraction (e.g., garbled Devanagari). Such chunks produce
 * meaningless embeddings and are better skipped than embedded.
 */
function isGarbled(text: string): boolean {
  if (text.length === 0) return true;
  // Indian legal acts never contain CJK text; any CJK characters are font-mapping mojibake.
  if (/[\u4e00-\u9fff]/.test(text)) return true;
  // Count control characters/replacements
  const garbageCount = (text.match(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F\uFFFD\uFFFE\uFFFF]/g) ?? []).length;
  return garbageCount / text.length > 0.05; // >5% garbage = skip
}

async function embedBatch(texts: string[]): Promise<number[][]> {
  const url = `${OLLAMA_URL}/api/embed`;

  // Sanitize (strip mojibake/control chars) then truncate to stay within token limit.
  const formattedTexts = texts.map((t) => {
    const clean = sanitizeText(t);
    const safe  = clean.length > MAX_CHARS ? clean.slice(0, MAX_CHARS) : clean;
    return `search_document: ${safe}`;
  });

  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      model: EMBEDDING_MODEL,
      input: formattedTexts,
    }),
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Ollama API error: ${response.statusText} - ${errorText}`);
  }

  const data = (await response.json()) as { embeddings: number[][] };
  return data.embeddings;
}

// ─── Upload ───────────────────────────────────────────────────────────────────

async function uploadChunks(chunks: Chunk[]): Promise<void> {
  let skipped  = 0;

  // Pre-filter garbled chunks (corrupted PDF extraction — mostly Hindi pages)
  const goodChunks = chunks.filter((c) => {
    if (isGarbled(c.text)) {
      console.log(`  [SKIP] Garbled chunk: ${c.chunk_id.slice(0, 60)}...`);
      skipped++;
      return false;
    }
    return true;
  });
  
  if (skipped > 0) {
    console.log(`\n  Filtered out ${skipped} garbled/mojibake chunks.`);
  }
  
  const total = goodChunks.length;
  console.log(`  Actual chunks to embed & upload: ${total}\n`);
  
  let uploaded = 0;
  for (let i = 0; i < total; i += BATCH_SIZE) {
    const batch  = goodChunks.slice(i, i + BATCH_SIZE);
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
  console.log(`IP-SAKTI  ·  Embed & Upload (Ollama)`);
  console.log(`Collection : ${COLLECTION_NAME}  |  Qdrant: ${QDRANT_URL}`);
  console.log(`Model      : ${EMBEDDING_MODEL} (dim=${VECTOR_SIZE})`);
  console.log(`${"─".repeat(50)}\n`);

  try {
    // 1. Ensure Qdrant collection exists with correct dimensions
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
