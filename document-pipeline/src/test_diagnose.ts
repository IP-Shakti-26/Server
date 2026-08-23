import "dotenv/config";
import fs from "fs";
import path from "path";
import { QdrantClient } from "@qdrant/js-client-rest";
import { Chunk } from "./types";

const OLLAMA_URL = process.env.OLLAMA_URL ?? "http://localhost:11434";
const QDRANT_URL = process.env.QDRANT_URL ?? "http://localhost:6333";
const COLLECTION_NAME = process.env.COLLECTION_NAME ?? "ipsakti_docs";
const EMBEDDING_MODEL = "nomic-embed-text:v1.5";

const qdrant = new QdrantClient({ url: QDRANT_URL });
const CHUNKS_DIR = path.resolve(__dirname, "../chunks");

function loadAllChunks(): Chunk[] {
  const chunkFiles = fs
    .readdirSync(CHUNKS_DIR)
    .filter((f) => f.endsWith("_chunks.json"));

  const allChunks: Chunk[] = [];
  for (const file of chunkFiles) {
    const raw = fs.readFileSync(path.join(CHUNKS_DIR, file), "utf-8");
    const chunks: Chunk[] = JSON.parse(raw);
    allChunks.push(...chunks);
  }
  return allChunks;
}

async function getExistingChunkIds(): Promise<Set<string>> {
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
    if (!res.next_page_offset) break;
    offset = res.next_page_offset;
  }
  return existingIds;
}

function sanitizeText(text: string): string {
  return text
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F\uFFFD]/g, " ")
    .replace(/\s{3,}/g, "  ")
    .trim();
}

function isGarbled(text: string): boolean {
  if (text.length === 0) return true;
  // Indian legal acts never contain CJK text; any CJK characters are font-mapping mojibake.
  if (/[\u4e00-\u9fff]/.test(text)) return true;
  const garbageCount = (text.match(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F\uFFFD\uFFFE\uFFFF]/g) ?? []).length;
  return garbageCount / text.length > 0.05;
}

async function testEmbed(text: string): Promise<boolean> {
  const clean = sanitizeText(text);
  // Using 7500 chars limit (same as embed_upload.ts)
  const safe = clean.length > 7500 ? clean.slice(0, 7500) : clean;
  const prompt = `search_document: ${safe}`;

  try {
    const response = await fetch(`${OLLAMA_URL}/api/embed`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model: EMBEDDING_MODEL,
        input: [prompt],
      }),
    });
    if (!response.ok) {
      const errorText = await response.text();
      console.log(`Failed (HTTP ${response.status}): ${errorText}`);
      return false;
    }
    return true;
  } catch (e: any) {
    console.log(`Failed (Fetch error): ${e.message}`);
    return false;
  }
}

async function main() {
  console.log("Loading chunks...");
  const allChunks = loadAllChunks();
  const existing = await getExistingChunkIds();
  const remaining = allChunks.filter(c => !existing.has(c.chunk_id));
  console.log(`Remaining chunks to check: ${remaining.length}`);

  const goodRemaining = remaining.filter(c => {
    if (isGarbled(c.text)) {
      console.log(`Skipping garbled chunk: ${c.chunk_id}`);
      return false;
    }
    return true;
  });
  console.log(`Remaining non-garbled chunks to test: ${goodRemaining.length}`);

  let failedCount = 0;
  const maxToTest = Math.min(goodRemaining.length, 50);
  console.log(`Testing first ${maxToTest} non-garbled chunks...`);
  for (let i = 0; i < maxToTest; i++) {
    const chunk = goodRemaining[i];
    console.log(`[${i + 1}/${maxToTest}] Testing ${chunk.chunk_id} (len: ${chunk.text.length})...`);
    const ok = await testEmbed(chunk.text);
    if (!ok) {
      failedCount++;
      console.log(`\n--- FAILED CHUNK #${failedCount} ---`);
      console.log(`ID: ${chunk.chunk_id}`);
      console.log(`Doc Title: ${chunk.doc_title}`);
      console.log(`Original Text Length: ${chunk.text.length}`);
      const clean = sanitizeText(chunk.text);
      console.log(`Cleaned Text Length: ${clean.length}`);

      // Let's print the first 300 characters showing their char codes
      const sample = clean.slice(0, 300);
      console.log("Sample characters and their charCodes:");
      let codes = [];
      for (let j = 0; j < sample.length; j++) {
        const code = sample.charCodeAt(j);
        codes.push(`${sample[j]} (${code})`);
      }
      console.log(codes.slice(0, 50).join(", ") + " ...");

      // Let's count non-ascii characters (charCodeAt > 127)
      let nonAscii = 0;
      for (let j = 0; j < clean.length; j++) {
        if (clean.charCodeAt(j) > 127) nonAscii++;
      }
      console.log(`Non-ASCII characters: ${nonAscii} (${(nonAscii / clean.length * 100).toFixed(1)}%)`);

      if (failedCount >= 5) {
        console.log("\nFound 5 failures, stopping diagnostic.");
        break;
      }
    }
  }
  console.log("\nDiagnostic run complete.");
}

main();
